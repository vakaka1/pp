package config

const (
	DefaultFallbackPublishMinDelayMinutes = 15
	DefaultFallbackPublishMaxDelayMinutes = 75
)

// ResolveFallbackPublishWindow returns the randomized publication window in minutes.
// The legacy publish_interval_minutes field is still accepted and translated into
// a jittered range so older configs do not keep a rigid schedule forever.
func ResolveFallbackPublishWindow(settings *FallbackSettings) (int, int) {
	if settings == nil {
		return DefaultFallbackPublishMinDelayMinutes, DefaultFallbackPublishMaxDelayMinutes
	}

	minDelay := settings.PublishMinDelayMinutes
	maxDelay := settings.PublishMaxDelayMinutes
	legacyInterval := settings.PublishIntervalMinutes

	switch {
	case minDelay > 0 && maxDelay > 0:
		if maxDelay < minDelay {
			maxDelay = minDelay
		}
		return minDelay, maxDelay
	case minDelay > 0:
		if maxDelay <= 0 {
			if legacyInterval > 0 && legacyInterval >= minDelay {
				maxDelay = legacyInterval
			} else {
				maxDelay = minDelay + maxInt(10, minDelay/2)
			}
		}
		if maxDelay < minDelay {
			maxDelay = minDelay
		}
		return minDelay, maxDelay
	case maxDelay > 0:
		if minDelay <= 0 {
			if legacyInterval > 0 && legacyInterval <= maxDelay {
				minDelay = legacyInterval
			} else {
				minDelay = maxInt(5, maxDelay*2/3)
			}
		}
		if minDelay > maxDelay {
			minDelay = maxDelay
		}
		return minDelay, maxDelay
	case legacyInterval > 0:
		minDelay = maxInt(5, legacyInterval*3/5)
		maxDelay = maxInt(minDelay, legacyInterval*3/2)
		return minDelay, maxDelay
	default:
		return DefaultFallbackPublishMinDelayMinutes, DefaultFallbackPublishMaxDelayMinutes
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
