package executor

import (
	"bytes"
	"fmt"
	"io"

	"github.com/tidwall/gjson"
)

func WriteSuccessResponse(out io.Writer, body []byte) error {
	if len(bytes.TrimSpace(body)) == 0 {
		return fmt.Errorf("response body is empty")
	}
	if !gjson.ValidBytes(body) {
		return fmt.Errorf("response body is not valid JSON")
	}
	_, err := out.Write(body)
	return err
}
