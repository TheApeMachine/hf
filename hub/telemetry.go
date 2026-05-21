package hub

import (
	"time"

	"github.com/theapemachine/qpool"
)

func publishHubProgress(op string, message string, fields ...qpool.Field) {
	event := qpool.NewInfoEvent("hub", op, message, fields)
	event.WithTime(time.Now())
	qpool.Publish(event)
}

func shouldPublishHubProgress(index int, total int) bool {
	return index == 0 || index+1 == total || (index+1)%64 == 0
}
