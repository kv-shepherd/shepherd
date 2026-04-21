package jobs

import (
	"context"
	stderrors "errors"
	"net"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

const clusterRuntimeUnavailableSnoozeDuration = 5 * time.Minute

func isClusterRuntimeUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if appErr, ok := apperrors.IsAppError(err); ok {
		return appErr.HTTPStatus == 503 || appErr.Code == apperrors.CodeClusterUnhealthy
	}
	if stderrors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error
	if stderrors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	if apierrors.IsTimeout(err) ||
		apierrors.IsServerTimeout(err) ||
		apierrors.IsServiceUnavailable(err) ||
		apierrors.IsUnexpectedServerError(err) ||
		apierrors.IsTooManyRequests(err) {
		return true
	}

	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"not healthy",
		"apiserver unreachable",
		"kubeconfig is empty",
		"no configuration has been provided",
		"invalid configuration",
		"connection refused",
		"dial tcp",
		"i/o timeout",
		"tls handshake timeout",
		"no such host",
		"server misbehaving",
		"client.timeout exceeded",
		"context deadline exceeded",
		"x509:",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func snoozeClusterRuntimeUnavailable(jobKind, eventID, clusterID, stage string, err error) error {
	logger.Warn("job snoozed due to cluster runtime unavailability",
		zap.String("job_kind", jobKind),
		zap.String("event_id", eventID),
		zap.String("cluster_id", strings.TrimSpace(clusterID)),
		zap.String("stage", stage),
		zap.Duration("snooze_for", clusterRuntimeUnavailableSnoozeDuration),
		zap.Error(err),
	)
	return river.JobSnooze(clusterRuntimeUnavailableSnoozeDuration)
}
