package lan

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"

	"github.com/dawsonxiong/thock/internal/race"
)

// maxLine caps one message. The largest real one is the results of a full
// room, every racer's keystrokes included, at well under a megabyte.
const maxLine = 4 << 20

// reader decodes newline-delimited messages.
type reader struct{ s *bufio.Scanner }

func newReader(r io.Reader) *reader {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 64<<10), maxLine)
	return &reader{s: s}
}

func (r *reader) read() (race.Msg, error) {
	var m race.Msg
	if !r.s.Scan() {
		if err := r.s.Err(); err != nil {
			return m, err
		}
		return m, io.EOF
	}
	if err := json.Unmarshal(r.s.Bytes(), &m); err != nil {
		return m, err
	}
	if m.T == "" {
		return m, errors.New("message with no type")
	}
	return m, nil
}

// encode renders one message as a line.
func encode(m race.Msg) ([]byte, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
