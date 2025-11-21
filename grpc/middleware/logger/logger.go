//nolint:revive // package name uses underscore for consistency with project structure
package grpc_logger

import (
	"context"
	"log/slog"

	"github.com/shortlink-org/go-sdk/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func printLog(ctx context.Context, log logger.Logger, err error, fields ...slog.Attr) {
	switch status.Code(err) {
	case
		codes.OK,
		codes.Canceled,
		codes.InvalidArgument,
		codes.NotFound,
		codes.AlreadyExists,
		codes.ResourceExhausted,
		codes.FailedPrecondition,
		codes.Aborted,
		codes.OutOfRange:
		log.DebugWithContext(ctx, err.Error(), fields...)
	case codes.Unknown, codes.DeadlineExceeded, codes.PermissionDenied, codes.Unauthenticated:
		log.InfoWithContext(ctx, err.Error(), fields...)
	case codes.Unimplemented, codes.Internal, codes.Unavailable, codes.DataLoss:
		log.WarnWithContext(ctx, err.Error(), fields...)
	default:
		log.InfoWithContext(ctx, err.Error(), fields...)
	}
}
