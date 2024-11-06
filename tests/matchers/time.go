package matchers

import (
	"fmt"
	"time"

	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
)

type representsTimeMatcher struct {
	t time.Time
}

func RepresentsTime(t time.Time) types.GomegaMatcher {
	return &representsTimeMatcher{
		t: t,
	}
}

func (m *representsTimeMatcher) Match(actual interface{}) (bool, error) {
	actualTimeString, ok := actual.(string)
	if !ok {
		return false, fmt.Errorf("%#v is not a string", actual)
	}

	actualTime, err := time.Parse(time.RFC3339, actualTimeString)
	if err != nil {
		return false, err
	}

	return actualTime.Equal(m.t), nil
}

func (m *representsTimeMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, fmt.Sprintf("to represent time %q ", m.t))
}

func (m *representsTimeMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, fmt.Sprintf("not to represent time %q ", m.t))
}
