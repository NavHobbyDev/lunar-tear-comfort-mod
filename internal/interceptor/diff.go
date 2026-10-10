package interceptor

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"runtime/debug"
	"sort"
	"strings"
	"sync"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/service"
	"lunar-tear/server/internal/store"
	"lunar-tear/server/internal/userdata"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type diffUserDataGetter interface {
	GetDiffUserData() map[string]*pb.DiffData
}

var (
	diffFieldCache  sync.Map // map[reflect.Type]bool
	diffMethodCache sync.Map // map[string]bool  (method name → resp has DiffUserData)
	diffMethodKnown sync.Map // map[string]struct{} (method name → we've inspected it)
	diffDataMapTyp  = reflect.TypeFor[map[string]*pb.DiffData]()
)

func hasDiffField(resp any) bool {
	t := reflect.TypeOf(resp)
	if cached, ok := diffFieldCache.Load(t); ok {
		return cached.(bool)
	}
	elem := t
	if elem.Kind() == reflect.Pointer {
		elem = elem.Elem()
	}
	f, ok := elem.FieldByName("DiffUserData")
	result := ok && f.Type == diffDataMapTyp
	diffFieldCache.Store(t, result)
	return result
}

func NewDiffInterceptor(
	users store.UserRepository,
	sessions store.SessionRepository,
	holder *runtime.Holder,
) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if skipDiffForMethod(info.FullMethod) {
			return handler(ctx, req)
		}

		// Fast path: if we've already inspected this method and it does NOT
		// have a DiffUserData field, skip the expensive LoadUser snapshots.
		if _, known := diffMethodKnown.Load(info.FullMethod); known {
			if isDiff, _ := diffMethodCache.Load(info.FullMethod); !isDiff.(bool) {
				return handler(ctx, req)
			}
		}

		userId := service.CurrentUserId(ctx, users, sessions)
		if userId == 0 {
			return handler(ctx, req)
		}

		// Snapshot before running the handler.
		before, err := users.LoadUser(userId)
		if err != nil {
			return handler(ctx, req)
		}
		
		// Check if daily refresh is needed (midnight crossed)
		if before.Login.LastLoginDatetime < gametime.StartOfDayMillisAt(gametime.NowMillis()) {
			if _, err := users.UpdateUser(userId, func(current *store.UserState) {
				service.RefreshDailyLoginState(current, gametime.NowMillis())
				// Reinitialize hidden category missions on daily refresh to keep them hidden
				service.InitializeHiddenCategoryMissions(current, holder.Get().Mission, gametime.NowMillis())
			}); err != nil {
				log.Printf("[DiffInterceptor] daily refresh failed for user=%d: %v", userId, err)
			}
		}

		resp, handlerErr := handler(ctx, req)
		if handlerErr != nil || resp == nil {
			return resp, handlerErr
		}

		needsDiff := hasDiffField(resp)

		// Cache the result for future calls.
		if _, alreadyKnown := diffMethodKnown.Load(info.FullMethod); !alreadyKnown {
			diffMethodCache.Store(info.FullMethod, needsDiff)
			diffMethodKnown.Store(info.FullMethod, struct{}{})
		}

		if !needsDiff {
			return resp, nil
		}

		if getter, ok := resp.(diffUserDataGetter); ok {
			if existing := getter.GetDiffUserData(); len(existing) > 0 {
				setUpdateNamesTrailer(ctx, existing)
				return resp, nil
			}
		}

		after, err := users.LoadUser(userId)
		if err != nil {
			return resp, nil
		}

		changed := userdata.ChangedTables(&before, &after)
		if len(changed) == 0 {
			return resp, nil
		}

		var diff map[string]*pb.DiffData
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[PANIC] DiffInterceptor ComputeDelta user=%d tables=%v: %v\n%s", userId, changed, r, debug.Stack())
					diff = nil
				}
			}()
			diff = userdata.ComputeDelta(&before, &after, changed)
		}()
		if diff == nil {
			return resp, fmt.Errorf("panic computing diff for tables %v", changed)
		}
		reflect.ValueOf(resp).Elem().FieldByName("DiffUserData").Set(reflect.ValueOf(diff))
		setUpdateNamesTrailer(ctx, diff)

		return resp, nil
	}
}

func skipDiffForMethod(method string) bool {
	switch method {
	case "/apb.api.user.UserService/Auth",
		"/apb.api.user.UserService/RegisterUser",
		"/apb.api.user.UserService/TransferUser",
		"/apb.api.user.UserService/TransferUserByFacebook",
		"/apb.api.config.ConfigService/GetConfig",
		"/apb.api.data.DataService/GetLatestMasterDataVersion",
		"/apb.api.data.DataService/GetUserDataNameV2",
		"/apb.api.data.DataService/GetUserData",
		// Read-only: the handler never mutates state, yet the response type
		// carries a DiffUserData field, so without this the interceptor pays
		// two extra LoadUser snapshots (~2x60ms) on every header refresh.
		// Daily-login refresh is already covered by GameStart and GetUserData.
		"/apb.api.notification.NotificationService/GetHeaderNotification",
		// UpdateSequence only ensures a gimmick-sequence row exists (no rewards,
		// no other tables) and is fired once per sequence in a burst on map load.
		// Skipping the diff avoids two full LoadUser snapshots per call; the
		// handler persists the row directly via EnsureGimmickSequence.
		"/apb.api.gimmick.GimmickService/UpdateSequence",
		// The Character Viewer top screen drives its "new locations" popup purely
		// from the releaseCharacterViewerFieldId list in the response, and the
		// handler persists the viewed state itself (so the badge never re-shows).
		// The viewer client cannot apply a live diffUserData delta for
		// IUserCharacterViewerField and crashes on it, so this RPC must return the
		// id list with NO table diff attached.
		"/apb.api.characterviewer.CharacterViewerService/CharacterViewerTop":
		return true
	}
	return false
}

func setUpdateNamesTrailer(ctx context.Context, diff map[string]*pb.DiffData) {
	if len(diff) == 0 {
		return
	}
	keys := make([]string, 0, len(diff))
	for key := range diff {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	value := strings.Join(keys, ",")
	if err := grpc.SetTrailer(ctx, metadata.Pairs("x-apb-update-user-data-names", value)); err != nil {
		log.Printf("[DiffInterceptor] failed to set trailer: %v", err)
	}
}
