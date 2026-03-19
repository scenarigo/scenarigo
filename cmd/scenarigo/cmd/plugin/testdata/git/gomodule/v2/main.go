package gomodule

import (
	"fmt"

	"127.0.0.1/dependent-gomodule.git"
)

var Dependency = fmt.Sprintf("gomodule => %s", dependent.Dependency)
