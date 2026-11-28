package submissions

import (
	"github.com/standards-lab/docinfer"
)

type extracts struct {
	Extract  *docinfer.Extract `json:"extract,omitempty"`
	Document *docinfer.Extract `json:"document,omitempty"`
	Timeline *docinfer.Extract `json:"timeline,omitempty"`
}
