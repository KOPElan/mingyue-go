package share

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	apperrors "kopelan/mingyue-go/internal/errors"
)

// SambaUser represents a Samba user account managed by smbpasswd/pdbedit.
type SambaUser struct {
	// Username is the Linux username registered in the Samba database.
	Username string `json:"username"`
}

// SambaUserCommander runs commands with optional stdin input.
// It is a superset of Commander, supporting smbpasswd which reads passwords
// from stdin when invoked with the -s flag.
type SambaUserCommander interface {
	// Run executes a command and returns combined output.
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
	// RunWithInput executes a command with the provided string written to stdin.
	RunWithInput(ctx context.Context, stdin string, name string, args ...string) ([]byte, error)
}

// SambaUserManager manages Samba user accounts via smbpasswd and pdbedit.
type SambaUserManager struct {
	commander SambaUserCommander
}

// NewSambaUserManager returns a SambaUserManager that calls real system binaries.
func NewSambaUserManager() *SambaUserManager {
	return &SambaUserManager{commander: &osSambaCommander{}}
}

// NewSambaUserManagerWithCommander returns a SambaUserManager with an injected
// commander, primarily for unit tests.
func NewSambaUserManagerWithCommander(cmd SambaUserCommander) *SambaUserManager {
	return &SambaUserManager{commander: cmd}
}

// ListUsers returns all users registered in the Samba database.
// It calls "pdbedit -L" and parses the "username:uid:comment" output format.
func (m *SambaUserManager) ListUsers(ctx context.Context) ([]SambaUser, error) {
	out, err := m.commander.Run(ctx, "pdbedit", "-L")
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternal, "failed to list samba users", err)
	}
	var users []SambaUser
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// pdbedit -L output: "username:uid:full name"
		parts := strings.SplitN(line, ":", 2)
		if parts[0] != "" {
			users = append(users, SambaUser{Username: parts[0]})
		}
	}
	return users, nil
}

// AddUser adds username to the Samba database and sets its initial password.
// Internally calls "smbpasswd -a -s <username>" with the password sent twice on stdin.
func (m *SambaUserManager) AddUser(ctx context.Context, username, password string) error {
	if strings.TrimSpace(username) == "" {
		return apperrors.New(apperrors.ErrInvalidInput, "username must not be empty")
	}
	// smbpasswd -a -s reads: new password, then confirmation password.
	input := password + "\n" + password + "\n"
	if _, err := m.commander.RunWithInput(ctx, input, "smbpasswd", "-a", "-s", username); err != nil {
		return apperrors.Wrap(apperrors.ErrInternal,
			fmt.Sprintf("failed to add samba user %q", username), err)
	}
	return nil
}

// RemoveUser removes username from the Samba database.
// Internally calls "smbpasswd -x <username>".
func (m *SambaUserManager) RemoveUser(ctx context.Context, username string) error {
	if strings.TrimSpace(username) == "" {
		return apperrors.New(apperrors.ErrInvalidInput, "username must not be empty")
	}
	if _, err := m.commander.Run(ctx, "smbpasswd", "-x", username); err != nil {
		return apperrors.Wrap(apperrors.ErrInternal,
			fmt.Sprintf("failed to remove samba user %q", username), err)
	}
	return nil
}

// SetPassword changes the password for an existing Samba user.
// Internally calls "smbpasswd -s <username>" with the new password on stdin.
func (m *SambaUserManager) SetPassword(ctx context.Context, username, password string) error {
	if strings.TrimSpace(username) == "" {
		return apperrors.New(apperrors.ErrInvalidInput, "username must not be empty")
	}
	// smbpasswd -s reads: new password, then confirmation password.
	input := password + "\n" + password + "\n"
	if _, err := m.commander.RunWithInput(ctx, input, "smbpasswd", "-s", username); err != nil {
		return apperrors.Wrap(apperrors.ErrInternal,
			fmt.Sprintf("failed to set password for samba user %q", username), err)
	}
	return nil
}

// ── production commander ──────────────────────────────────────────────────────

// osSambaCommander runs real system commands; RunWithInput pipes a string to stdin.
type osSambaCommander struct{}

func (c *osSambaCommander) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func (c *osSambaCommander) RunWithInput(ctx context.Context, stdin string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	return cmd.CombinedOutput()
}
