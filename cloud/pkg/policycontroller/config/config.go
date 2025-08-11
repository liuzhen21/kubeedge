package config

import (
	"sync"

	v1alpha1 "github.com/kubeedge/api/apis/componentconfig/cloudcore/v1alpha1"
)

var (
	Config Configure
	once   sync.Once
)

type Configure struct {
	v1alpha1.PolicyController
}

func InitConfigure(pc *v1alpha1.PolicyController) {
	if pc == nil {
		// keep zero value defaults if not provided
		return
	}
	once.Do(func() {
		Config = Configure{
			PolicyController: *pc,
		}
	})
}
