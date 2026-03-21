package tools

import (
	"fmt"
	"strconv"
	"strings"
)

type StringInteger float64

func (d *StringInteger) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*d = StringInteger(v)
	return nil
}

type StringFloat float64

func (d *StringFloat) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*d = StringFloat(v)
	return nil
}

type StringFramerate float64

func (d *StringFramerate) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)

	if parts := strings.SplitN(s, "/", 2); len(parts) == 2 {

		num, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return fmt.Errorf("invalid numerator: %s", err)
		}
		den, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return fmt.Errorf("invalid denominator: %s", err)
		}

		if den == 0 {
			den = 1
		}

		*d = StringFramerate(num / den)
		return nil
	}

	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("cannot parse framerate")
	}
	*d = StringFramerate(f)

	return nil
}

type ProbeStream struct {
	Index        int             `json:"index"`        // 0
	CodecType    string          `json:"codec_type"`   // video
	Width        int             `json:"width"`        // 1920
	Height       int             `json:"height"`       // 1080
	NumberFrames StringInteger   `json:"nb_frames"`    // 1
	RFrameRate   StringFramerate `json:"r_frame_rate"` // 15/1
	Duration     StringFloat     `json:"duration"`     // 251.800
}

type ProbeResults struct {
	Streams []ProbeStream `json:"streams"`
}
