package version

import (
	"fmt"
)

var (
	Name      = "unset"
	GitCommit = "unset"
	Version   string

	HumanVersion = fmt.Sprintf("%s v'%s' (%s)", Name, Version, GitCommit)
)
