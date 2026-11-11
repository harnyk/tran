package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/harnyk/tran/internal/converter"
	"github.com/harnyk/tran/internal/meeting"
)

var soundmixCmd = &cobra.Command{
	Use:   "soundmix",
	Short: "Mix source audio tracks and segment into MP3 chunks",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		m, err := meeting.Detect(cwd)
		if err != nil {
			return err
		}

		micDB, _ := cmd.Flags().GetFloat64("mic-volume")
		sysDB, _ := cmd.Flags().GetFloat64("sys-volume")
		warnVolumeIgnored(m, micDB, sysDB)

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		fmt.Println("Mixing audio...")
		result, err := conv.Convert(ctx, m, converter.Options{
			MicVolumeDB: micDB,
			SysVolumeDB: sysDB,
		})
		if err != nil {
			return err
		}

		fmt.Printf("Created %d segment(s):\n", len(result.Segments))
		for _, s := range result.Segments {
			info, _ := os.Stat(s)
			if info != nil {
				fmt.Printf("  %s (%.1f MB)\n", s, float64(info.Size())/1024/1024)
			} else {
				fmt.Printf("  %s\n", s)
			}
		}
		return nil
	},
}

func init() {
	soundmixCmd.Flags().Float64("mic-volume", 0, "microphone volume adjustment in dB (e.g. 3 or -6)")
	soundmixCmd.Flags().Float64("sys-volume", 0, "system audio volume adjustment in dB (e.g. 3 or -6)")
	rootCmd.AddCommand(soundmixCmd)
}

// warnVolumeIgnored tells the user that per-channel volume flags have no effect
// when the meeting has a single recording instead of mic.mp3 + sys.mp3.
func warnVolumeIgnored(m *meeting.MeetingDir, micDB, sysDB float64) {
	if (micDB != 0 || sysDB != 0) && !m.HasDualSources() {
		fmt.Println("Note: --mic-volume/--sys-volume are ignored for a single-source recording")
	}
}
