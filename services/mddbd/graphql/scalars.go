package graphql

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/99designs/gqlgen/graphql"
)

// MarshalTime marshals time.Time to Unix timestamp
func MarshalTime(t time.Time) graphql.Marshaler {
	if t.IsZero() {
		return graphql.Null
	}

	return graphql.WriterFunc(func(w io.Writer) {
		_, _ = io.WriteString(w, strconv.FormatInt(t.Unix(), 10))
	})
}

// UnmarshalTime unmarshals Unix timestamp to time.Time
func UnmarshalTime(v interface{}) (time.Time, error) {
	if tmpStr, ok := v.(string); ok {
		i, err := strconv.ParseInt(tmpStr, 10, 64)
		if err != nil {
			return time.Time{}, err
		}
		return time.Unix(i, 0), nil
	}

	if tmpInt, ok := v.(int64); ok {
		return time.Unix(tmpInt, 0), nil
	}

	if tmpInt, ok := v.(int); ok {
		return time.Unix(int64(tmpInt), 0), nil
	}

	return time.Time{}, fmt.Errorf("unable to unmarshal Time from %T", v)
}
