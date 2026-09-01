//nolint:revive // package name uses underscore for consistency with project structure
package grpc_logger

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func printLog(ctx context.Context, log *slog.Logger, err error, fields ...slog.Attr) {
	log.LogAttrs(ctx, levelFor(err), err.Error(), fields...)
}

// levelFor grades a gRPC status: the codes a healthy service returns as part
// of its contract stay at debug, and only the ones that point at a defect
// climb to warn
func levelFor(err error) slog.Level {
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
		return slog.LevelDebug
	case codes.Unknown, codes.DeadlineExceeded, codes.PermissionDenied, codes.Unauthenticated:
		return slog.LevelInfo
	case codes.Unimplemented, codes.Internal, codes.Unavailable, codes.DataLoss:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}
