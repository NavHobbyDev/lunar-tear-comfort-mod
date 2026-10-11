package userdata

import (
	pb "lunar-tear/server/gen/proto"
)

func EmptyDiff() map[string]*pb.DiffData {
	return map[string]*pb.DiffData{}
}
