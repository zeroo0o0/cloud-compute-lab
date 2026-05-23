package cluster

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

const (
	PodDeletionCostAnnotation = "controller.kubernetes.io/pod-deletion-cost"
	ActivePlayersAnnotation   = "lab4/active-players"
	DrainingAnnotation        = "lab4/draining"
)

func StartPodDeletionCostReporter(ctx context.Context, namespace string, activePlayers func() int, draining func() bool) error {
	client, err := NewInClusterClient(namespace)
	if err != nil {
		return err
	}
	report := func() {
		active := safeActivePlayers(activePlayers)
		isDraining := safeDraining(draining)
		cost := PodDeletionCost(active, isDraining)
		_ = client.PatchOwnPodAnnotations(ctx, map[string]string{
			PodDeletionCostAnnotation: strconv.Itoa(cost),
			ActivePlayersAnnotation:   strconv.Itoa(active),
			DrainingAnnotation:        strconv.FormatBool(isDraining),
		})
	}
	report()
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				report()
			case <-ctx.Done():
				return
			}
		}
	}()
	return nil
}

func PodDeletionCost(activePlayers int, draining bool) int {
	if activePlayers <= 0 {
		if draining {
			return -2000
		}
		return -1000
	}
	if activePlayers > 80 {
		activePlayers = 80
	}
	return 1000 + activePlayers*100
}

func safeActivePlayers(fn func() int) (active int) {
	defer func() {
		if recover() != nil {
			active = 0
		}
	}()
	if fn == nil {
		return 0
	}
	active = fn()
	if active < 0 {
		return 0
	}
	return active
}

func safeDraining(fn func() bool) (draining bool) {
	defer func() {
		if recover() != nil {
			draining = false
		}
	}()
	return fn != nil && fn()
}

func PodDeletionCostText(activePlayers int, draining bool) string {
	return fmt.Sprintf("%d", PodDeletionCost(activePlayers, draining))
}
