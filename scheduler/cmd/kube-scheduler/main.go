package main

import (
	"os"

	"k8s.io/component-base/logs"
	"k8s.io/kubernetes/cmd/kube-scheduler/app"

	"github.com/georgelipceanu/susk8s/scheduler/pkg/framework/plugins/carbonbinpack"
	"github.com/georgelipceanu/susk8s/scheduler/pkg/framework/plugins/carbonfilter"
)

func main() {
	cmd := app.NewSchedulerCommand(
		app.WithPlugin(carbonbinpack.Name, carbonbinpack.New),
		app.WithPlugin(carbonfilter.Name, carbonfilter.New),
	)

	logs.InitLogs()
	defer logs.FlushLogs()

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
