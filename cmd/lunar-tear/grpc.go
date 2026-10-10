package main

import (
	"log"
	"net"
	"strconv"
	"time"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/interceptor"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/service"
	"lunar-tear/server/internal/store"
	"lunar-tear/server/internal/userdata"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
)

type loggingListener struct {
	net.Listener
}

func (l loggingListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		log.Printf("[gRPC] Accept error: %v", err)
		return nil, err
	}
	log.Printf("[gRPC] New connection from %v", conn.RemoteAddr())
	return conn, nil
}

func startGRPC(
	listenAddr string,
	publicAddr string,
	octoURL string,
	authURL string,
	userStore interface {
		store.UserRepository
		store.SessionRepository
		store.SnapshotRepository
	},
	holder *runtime.Holder,
	noRegister bool,
) *grpc.Server {
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", listenAddr, err)
	}
	lis = loggingListener{Listener: lis}

	diffInterceptor := interceptor.NewDiffInterceptor(userStore, userStore, holder)
	sessionGuard := interceptor.NewSessionGuard(userStore)
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(interceptor.Platform, interceptor.Logging, sessionGuard, diffInterceptor, interceptor.TimeSync),
		grpc.UnknownServiceHandler(interceptor.UnknownService),
		// The default enforcement policy GOAWAYs ("too_many_pings") any client
		// that pings more often than every 5 minutes. Mobile clients ping much
		// more frequently, so the connection gets killed mid-session and the
		// client hangs on "connecting to server" while it reconnects with
		// backoff. Accept frequent pings and probe dead connections ourselves.
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             10 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    30 * time.Second,
			Timeout: 10 * time.Second,
		}),
	)

	registerServices(grpcServer, publicAddr, octoURL, authURL, userStore, holder, noRegister)

	reflection.Register(grpcServer)

	log.Printf("gRPC server listening on %s", lis.Addr())
	log.Printf("public address: %s", publicAddr)

	if noRegister {
		log.Print("[!!WARNING!!] The gRPC server is running in NO-REGISTER mode. All new user registrations are denied, only existing accounts and auth-server logins are permitted.")
	}

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Printf("gRPC server stopped: %v", err)
		}
	}()
	return grpcServer
}

func registerServices(
	srv *grpc.Server,
	publicAddr string,
	octoURL string,
	authURL string,
	userStore interface {
		store.UserRepository
		store.SessionRepository
		store.SnapshotRepository
	},
	holder *runtime.Holder,
	noRegister bool,
) {
	pubHost, pubPortStr, _ := net.SplitHostPort(publicAddr)
	pubPort, _ := strconv.Atoi(pubPortStr)

	// Inject the pre-loaded mission catalog so projectors don't trigger a second load.
	if cats := holder.Get(); cats != nil && cats.Mission != nil {
		userdata.SetMissionCatalog(cats.Mission)
	}

	directory := service.NewPlayerDirectory(userStore, holder)
	// Shared across the Character Viewer (marks the session on open) and Battle
	// (accepts the preview's quest-less StartWave) so the 3D team viewer works
	// while the orphan-wave guard still blocks genuine quest-less waves.
	viewerSessions := service.NewViewerSessions()
	pb.RegisterBannerServiceServer(srv, service.NewBannerServiceServer(holder, userStore, userStore))
	pb.RegisterUserServiceServer(srv, service.NewUserServiceServer(userStore, userStore, holder, authURL, noRegister, userStore, directory))
	pb.RegisterBattleServiceServer(srv, service.NewBattleServiceServer(userStore, userStore, holder, viewerSessions))
	pb.RegisterConfigServiceServer(srv, service.NewConfigServiceServer(pubHost, int32(pubPort), octoURL))
	pb.RegisterDataServiceServer(srv, service.NewDataServiceServer(userStore, userStore, holder))
	pb.RegisterTutorialServiceServer(srv, service.NewTutorialServiceServer(userStore, userStore, userStore, holder))
	pb.RegisterGachaServiceServer(srv, service.NewGachaServiceServer(userStore, userStore, holder))
	pb.RegisterGiftServiceServer(srv, service.NewGiftServiceServer(userStore, userStore, holder))
	pb.RegisterGamePlayServiceServer(srv, service.NewGameplayServiceServer())
	pb.RegisterGimmickServiceServer(srv, service.NewGimmickServiceServer(userStore, userStore, holder))
	pb.RegisterQuestServiceServer(srv, service.NewQuestServiceServer(userStore, userStore, holder))
	pb.RegisterNotificationServiceServer(srv, service.NewNotificationServiceServer(userStore, userStore))
	pb.RegisterCageOrnamentServiceServer(srv, service.NewCageOrnamentServiceServer(userStore, userStore, holder))
	pb.RegisterDeckServiceServer(srv, service.NewDeckServiceServer(userStore, userStore, userStore, holder))
	service.BackfillSnapshots(userStore, userStore)
	service.SeedArenaDecks(userStore, userStore)
	service.SeedBotRoster(directory, userStore)
	service.BackfillCostumeLevelBonuses(userStore, userStore, holder)
	pb.RegisterFriendServiceServer(srv, service.NewFriendServiceServer(userStore, userStore, directory, holder))
	pb.RegisterPvpServiceServer(srv, service.NewPvpServiceServer(userStore, userStore, userStore, directory, holder))
	pb.RegisterLoginBonusServiceServer(srv, service.NewLoginBonusServiceServer(userStore, userStore, userStore, holder))
	pb.RegisterNaviCutInServiceServer(srv, service.NewNaviCutInServiceServer(userStore, userStore))
	pb.RegisterContentsStoryServiceServer(srv, service.NewContentsStoryServiceServer(userStore, userStore))
	pb.RegisterDokanServiceServer(srv, service.NewDokanServiceServer(userStore, userStore))
	pb.RegisterPortalCageServiceServer(srv, service.NewPortalCageServiceServer(userStore, userStore))
	pb.RegisterCharacterViewerServiceServer(srv, service.NewCharacterViewerServiceServer(userStore, userStore, holder, viewerSessions))
	pb.RegisterMissionServiceServer(srv, service.NewMissionServiceServer(userStore, userStore, holder))
	pb.RegisterShopServiceServer(srv, service.NewShopServiceServer(userStore, userStore, holder))
	pb.RegisterCostumeServiceServer(srv, service.NewCostumeServiceServer(userStore, userStore, holder))
	pb.RegisterMovieServiceServer(srv, service.NewMovieServiceServer(userStore, userStore))
	pb.RegisterOmikujiServiceServer(srv, service.NewOmikujiServiceServer(userStore, userStore, holder))
	pb.RegisterWeaponServiceServer(srv, service.NewWeaponServiceServer(userStore, userStore, holder))
	pb.RegisterExploreServiceServer(srv, service.NewExploreServiceServer(userStore, userStore, holder))
	pb.RegisterCharacterBoardServiceServer(srv, service.NewCharacterBoardServiceServer(userStore, userStore, holder))
	pb.RegisterPartsServiceServer(srv, service.NewPartsServiceServer(userStore, userStore, holder))
	pb.RegisterCharacterServiceServer(srv, service.NewCharacterServiceServer(userStore, userStore, holder))
	pb.RegisterCompanionServiceServer(srv, service.NewCompanionServiceServer(userStore, userStore, holder))
	pb.RegisterMaterialServiceServer(srv, service.NewMaterialServiceServer(userStore, userStore, holder))
	pb.RegisterConsumableItemServiceServer(srv, service.NewConsumableItemServiceServer(userStore, userStore, holder))
	pb.RegisterSideStoryQuestServiceServer(srv, service.NewSideStoryQuestServiceServer(userStore, userStore, holder))
	pb.RegisterBigHuntServiceServer(srv, service.NewBigHuntServiceServer(userStore, userStore, holder))
	pb.RegisterRewardServiceServer(srv, service.NewRewardServiceServer(userStore, userStore, holder))
	pb.RegisterLabyrinthServiceServer(srv, service.NewLabyrinthServiceServer(userStore, userStore, holder))
}
