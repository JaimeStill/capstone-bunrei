package submissions

import (
	"encoding/json"
	"time"

	"github.com/standards-lab/go-core/event"

	"github.com/JaimeStill/capstone-bunrei/domain/jobs"
)

func work(job string, now time.Time) (event.Event, error) {
	data, err := json.Marshal(jobs.Work{Job: job})
	if err != nil {
		return event.Event{}, err
	}
	return event.Event{
		ID:              job,
		Source:          jobs.Source,
		Type:            jobs.TypeWork,
		Subject:         job,
		Time:            now,
		DataContentType: "application/json",
		Data:            data,
		Extensions:      map[string]string{jobs.ExtCorrelation: job},
	}, nil
}
