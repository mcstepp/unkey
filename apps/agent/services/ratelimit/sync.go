package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/bufbuild/connect-go"
	ratelimitv1 "github.com/unkeyed/unkey/apps/agent/gen/proto/ratelimit/v1"
	"github.com/unkeyed/unkey/apps/agent/gen/proto/ratelimit/v1/ratelimitv1connect"
	"github.com/unkeyed/unkey/apps/agent/pkg/ratelimit"
)

func ratelimitNodeKey(identifier string, limit int64, duration int64) string {
	window := time.Now().UnixMilli() / duration
	return fmt.Sprintf("ratelimit:%s:%d:%d", identifier, window, limit)
}

func (s *service) runPushPullSync() {

	client := http.DefaultClient

	for e := range s.pushPullC {
		key := ratelimitNodeKey(e.identifier, e.limit, e.duration)
		peer, err := s.cluster.FindNode(key)
		if err != nil {
			s.logger.Error().Err(err).Str("key", key).Msg("unable to find responsible node")
			continue
		}

		if peer.Id == s.cluster.NodeId() {
			s.logger.Debug().Str("key", key).Msg("skipping push pull with self")
			continue
		}
		s.logger.Debug().Str("peer", peer.Id).Str("key", key).Msg("push pull with")

		c := ratelimitv1connect.NewRatelimitServiceClient(client, peer.RpcAddr)

		req := connect.NewRequest(&ratelimitv1.PushPullRequest{
			Identifier: e.identifier,
			Limit:      e.limit,
			Duration:   e.duration,
			Cost:       e.cost,
		})
		s.logger.Info().Interface("req", req).Msg("push pull request")
		req.Header().Set("Authorization", s.cluster.AuthToken())

		res, err := c.PushPull(context.Background(), req)

		if err != nil {
			s.logger.Error().Err(err).Str("peerId", peer.Id).Msg("failed to push pull")
			continue
		}
		s.logger.Info().Str("peerId", peer.Id).Str("key", key).Interface("res", res).Msg("push pull came back")

		err = s.ratelimiter.SetCurrent(ratelimit.SetCurrentRequest{
			Identifier:     e.identifier,
			Max:            e.limit,
			Current:        res.Msg.Current,
			RefillInterval: e.duration,
		})
		if err != nil {
			s.logger.Error().Err(err).Msg("failed to set current")
			continue
		}

	}

}
