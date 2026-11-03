package main

import (
	"os"

	"k8s.io/component-base/cli"
	"k8s.io/kubernetes/cmd/kube-scheduler/app"

	"github.com/fitzyracing1/earthenware-k8s/pkg/earthenware"
)

func main() {
	cmd := app.NewSchedulerCommand(app.WithPlugin(earthenware.Name, earthenware.New))
	os.Exit(cli.Run(cmd))
}
