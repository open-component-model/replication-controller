package main

import (
	"fmt"
	"log"
	"os"

	"github.com/Masterminds/semver/v3"

	"github.com/open-component-model/replication-controller/pkg/version"
)

func main() {
	if len(os.Args) <= 1 {
		log.Fatal("missing argument")
	}

	_ = semver.MustParse(version.ReleaseVersion)

	cmd := os.Args[1]

	switch cmd {
	case "print-version":
		fmt.Print(version.ReleaseVersion)
	case "print-rc-version":
		fmt.Printf("%s-%s", version.ReleaseVersion, version.ReleaseCandidate)
	}
}
