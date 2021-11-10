package bridge

import (
	"sort"

	"github.com/aws/aws-sdk-go/service/ssm"
)

func SortParameters(parameters []*ssm.Parameter) {
	sort.Slice(parameters, func(i, j int) bool {
		return *parameters[i].Name < *parameters[j].Name
	})
}
