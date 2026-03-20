package main

import (
	"errors"
	"os"

	"github.com/ramsesyok/runnora/cmd"
	"github.com/ramsesyok/runnora/internal/app"
)

func main() {
	if err := run(); err != nil {
		var appErr *app.AppError
		if errors.As(err, &appErr) {
			os.Exit(appErr.ExitCode)
		}
		os.Exit(1)
	}
}

func run() error {
	return cmd.Execute()
}
