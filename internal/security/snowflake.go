package security

import (
	"errors"
	"strconv"
)

func ParseSnowflake(s string) (uint64, error) {
	if s == "" {
		return 0, errors.New("empty snowflake")
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, errors.New("snowflake must be numeric")
		}
	}
	id, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, errors.New("invalid snowflake")
	}
	if id == 0 {
		return 0, errors.New("snowflake must be > 0")
	}
	return id, nil
}



