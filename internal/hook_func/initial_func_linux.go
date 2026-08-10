package hook_func

import (
	"context"
	"fmt"
	"os/user"

	"github.com/majianyu2007/nwafu-connect/configs"
	"github.com/majianyu2007/nwafu-connect/log"
)

func init() {
	RegisterInitialFunc("check tun mode cap", func(ctx context.Context, config configs.Config) error {
		if config.TUNMode && !config.BrowserMode {
			current, err := user.Current()
			if err != nil {
				return fmt.Errorf("identify current user: %w", err)
			}
			if current.Uid != "0" {
				log.Println("TUN mode detected, but the current user is not root. This may cause issues. If you encounter problems, please run the application using sudo.")
			}
		}
		return nil
	})
}
