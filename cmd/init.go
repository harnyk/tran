package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/harnyk/tran/internal/meeting"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Turn the current directory into a meeting directory (source/, mix/, transcript/)",
	Long: `Creates source/, mix/, and transcript/ in the current directory.

Use it for recordings made outside of 'tran rec': put a single audio or video
file into source/ and run 'tran process'.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		m, err := meeting.Init(cwd)
		if err != nil {
			return err
		}
		fmt.Printf("Initialized meeting directory: %s\n", m.Path)
		fmt.Println("Put a single audio/video file into source/, then run: tran process")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
