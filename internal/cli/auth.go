package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"kopelan/mingyue-go/internal/auth"
)

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage API keys for the mingyue agent",
		Long: `Generate, list, and revoke API keys used to authenticate
web frontends and other clients against the mingyue agent.

Keys are stored in ` + auth.DefaultKeystorePath + ` (mode 0600).
Use --keystore to override the path.`,
	}

	cmd.AddCommand(newAuthKeygenCmd())
	cmd.AddCommand(newAuthListCmd())
	cmd.AddCommand(newAuthRevokeCmd())

	return cmd
}

func newAuthKeygenCmd() *cobra.Command {
	var role, subject, keystorePath string

	cmd := &cobra.Command{
		Use:   "keygen",
		Short: "Generate and save a new API key",
		Long: `Generate a cryptographically secure random API key, persist it to
the keystore file, and print it.  Store the printed key safely — it
cannot be recovered afterwards.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if keystorePath == "" {
				keystorePath = auth.DefaultKeystorePath
			}

			r := auth.Role(role)
			switch r {
			case auth.RoleViewer, auth.RoleOperator, auth.RoleAdmin:
				// valid
			default:
				return fmt.Errorf("invalid role %q: must be viewer, operator, or admin", role)
			}

			key, err := auth.GenerateKey()
			if err != nil {
				return err
			}

			entry := auth.KeyEntry{
				Key:       key,
				Role:      r,
				Subject:   subject,
				CreatedAt: time.Now().UTC(),
			}

			if err := auth.SaveKeyEntry(keystorePath, entry); err != nil {
				return fmt.Errorf("save key: %w", err)
			}

			if IsJSONOutput() {
				return WriteJSON(entry)
			}
			fmt.Printf("API key generated successfully:\n")
			fmt.Printf("  Key:     %s\n", key)
			fmt.Printf("  Role:    %s\n", role)
			fmt.Printf("  Subject: %s\n", subject)
			fmt.Printf("\nKeep this key secret — it grants %s access to the agent.\n", role)
			return nil
		},
	}

	cmd.Flags().StringVar(&role, "role", "viewer", "role to assign: viewer, operator, admin")
	cmd.Flags().StringVar(&subject, "subject", "default", "label identifying the key owner (e.g. web-frontend)")
	cmd.Flags().StringVar(&keystorePath, "keystore", "", "keystore file path (default: "+auth.DefaultKeystorePath+")")
	return cmd
}

func newAuthListCmd() *cobra.Command {
	var keystorePath string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List stored API keys (keys are partially masked)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if keystorePath == "" {
				keystorePath = auth.DefaultKeystorePath
			}

			entries, err := auth.ListKeyEntries(keystorePath)
			if err != nil {
				return err
			}

			if IsJSONOutput() {
				type maskedEntry struct {
					Key       string    `json:"key"`
					Role      auth.Role `json:"role"`
					Subject   string    `json:"subject"`
					CreatedAt time.Time `json:"created_at"`
				}
				masked := make([]maskedEntry, len(entries))
				for i, e := range entries {
					masked[i] = maskedEntry{
						Key:       maskKey(e.Key),
						Role:      e.Role,
						Subject:   e.Subject,
						CreatedAt: e.CreatedAt,
					}
				}
				return WriteJSON(masked)
			}

			if len(entries) == 0 {
				fmt.Println("No API keys found. Use 'mingyue auth keygen' to create one.")
				return nil
			}

			fmt.Printf("%-18s  %-10s  %-24s  %s\n", "KEY (masked)", "ROLE", "SUBJECT", "CREATED (UTC)")
			fmt.Printf("%-18s  %-10s  %-24s  %s\n",
				"------------------", "----------", "------------------------", "-------------------")
			for _, e := range entries {
				fmt.Printf("%-18s  %-10s  %-24s  %s\n",
					maskKey(e.Key), e.Role, e.Subject,
					e.CreatedAt.Format("2006-01-02 15:04:05"))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&keystorePath, "keystore", "", "keystore file path (default: "+auth.DefaultKeystorePath+")")
	return cmd
}

func newAuthRevokeCmd() *cobra.Command {
	var keystorePath string

	cmd := &cobra.Command{
		Use:   "revoke <key>",
		Short: "Revoke an API key",
		Long:  `Remove an API key from the keystore and from the running agent's memory.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if keystorePath == "" {
				keystorePath = auth.DefaultKeystorePath
			}

			if err := auth.RevokeKey(keystorePath, args[0]); err != nil {
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{
					"status": "revoked",
					"key":    maskKey(args[0]),
				})
			}
			fmt.Printf("Revoked API key: %s\n", maskKey(args[0]))
			return nil
		},
	}

	cmd.Flags().StringVar(&keystorePath, "keystore", "", "keystore file path (default: "+auth.DefaultKeystorePath+")")
	return cmd
}

// maskKey returns a partially redacted version of key for display purposes.
func maskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "..." + key[len(key)-4:]
}
