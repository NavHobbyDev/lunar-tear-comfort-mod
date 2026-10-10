package model

import (
	"context"
	"strconv"

	"google.golang.org/grpc/metadata"
)

type ClientPlatform struct {
	OsType       int32 // 1=iOS, 2=Android
	PlatformType int32 // 1=AppStore, 2=GooglePlay, 8=Amazon
}

const (
	OsTypeIOS     int32 = 1
	OsTypeAndroid int32 = 2

	PlatformTypeAppStore        int32 = 1
	PlatformTypeGooglePlayStore int32 = 2
	PlatformTypeAmazonAppStore  int32 = 8
)

var DefaultPlatform = ClientPlatform{OsType: OsTypeAndroid, PlatformType: PlatformTypeGooglePlayStore}

type platformKey struct{}

func (p ClientPlatform) String() string {
	os := "unknown"
	switch p.OsType {
	case OsTypeIOS:
		os = "iOS"
	case OsTypeAndroid:
		os = "Android"
	}
	plat := "unknown"
	switch p.PlatformType {
	case PlatformTypeAppStore:
		plat = "AppStore"
	case PlatformTypeGooglePlayStore:
		plat = "GooglePlay"
	case PlatformTypeAmazonAppStore:
		plat = "Amazon"
	}
	return os + "/" + plat
}

func ClientPlatformFromHeaders(ctx context.Context) ClientPlatform {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return DefaultPlatform
	}

	p := DefaultPlatform
	if vals := md.Get("x-apb-os-type"); len(vals) > 0 {
		if v, err := strconv.ParseInt(vals[0], 10, 32); err == nil {
			p.OsType = int32(v)
		}
	}
	if vals := md.Get("x-apb-platform-type"); len(vals) > 0 {
		if v, err := strconv.ParseInt(vals[0], 10, 32); err == nil {
			p.PlatformType = int32(v)
		}
	}
	return p
}

func NewContextWithPlatform(ctx context.Context, p ClientPlatform) context.Context {
	return context.WithValue(ctx, platformKey{}, p)
}

func ClientPlatformFromContext(ctx context.Context) ClientPlatform {
	if p, ok := ctx.Value(platformKey{}).(ClientPlatform); ok {
		return p
	}
	return DefaultPlatform
}
