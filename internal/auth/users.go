// Package auth — user store manages admin user accounts.
// MVP: single admin user stored as a JSON file in config/.
package auth

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/liteploy/liteploy/internal/storage"
)

const usersFile = "config/users.json"

// User represents an admin user.
type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"` // bcrypt hash
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// UserStore persists and retrieves user accounts.
type UserStore struct {
	store  *storage.Store
	logger *slog.Logger
	mu     sync.RWMutex
	users  map[string]*User // keyed by username
}

// NewUserStore creates a UserStore, loading any existing users from disk.
func NewUserStore(store *storage.Store, logger *slog.Logger) (*UserStore, error) {
	if logger == nil {
		logger = slog.Default()
	}
	us := &UserStore{
		store:  store,
		logger: logger,
		users:  make(map[string]*User),
	}
	if err := us.load(); err != nil {
		return nil, err
	}
	return us, nil
}

// HasUsers reports whether any users exist (used to detect first-run).
func (us *UserStore) HasUsers() bool {
	us.mu.RLock()
	defer us.mu.RUnlock()
	return len(us.users) > 0
}

// CreateAdmin creates the initial admin user. Fails if any user already exists.
func (us *UserStore) CreateAdmin(username, password string) error {
	if len(username) < 3 || len(username) > 64 {
		return errors.New("username must be 3-64 characters")
	}

	hash, err := HashPassword(password)
	if err != nil {
		return err
	}

	us.mu.Lock()
	defer us.mu.Unlock()

	if len(us.users) > 0 {
		return errors.New("admin user already exists; use change password instead")
	}

	user := &User{
		ID:           "admin-001",
		Username:     username,
		PasswordHash: hash,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	us.users[username] = user
	if err := us.save(); err != nil {
		delete(us.users, username)
		return err
	}

	if us.logger != nil {
		us.logger.Info("admin user created")
	}
	return nil
}

// Authenticate checks username + password and returns the user on success.
// Returns an error if credentials are wrong or user not found.
func (us *UserStore) Authenticate(username, password string) (*User, error) {
	us.mu.RLock()
	user, ok := us.users[username]
	us.mu.RUnlock()

	if !ok {
		// Use a constant-time fake check to prevent username enumeration timing.
		_, _ = HashPassword("dummyPassword123")
		return nil, errors.New("invalid credentials")
	}

	if !CheckPassword(password, user.PasswordHash) {
		return nil, errors.New("invalid credentials")
	}

	cp := *user
	return &cp, nil
}

// ChangePassword updates the password for an existing user.
func (us *UserStore) ChangePassword(username, oldPassword, newPassword string) error {
	us.mu.Lock()
	defer us.mu.Unlock()

	user, ok := us.users[username]
	if !ok {
		return errors.New("user not found")
	}
	if !CheckPassword(oldPassword, user.PasswordHash) {
		return errors.New("invalid current password")
	}

	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}

	user.PasswordHash = hash
	user.UpdatedAt = time.Now().UTC()
	return us.save()
}

// save persists the user list atomically. Called with mu held.
func (us *UserStore) save() error {
	list := make([]*User, 0, len(us.users))
	for _, u := range us.users {
		list = append(list, u)
	}
	if err := us.store.WriteJSON(usersFile, list); err != nil {
		return fmt.Errorf("save users: %w", err)
	}
	return nil
}

// load reads users from disk. Called once at startup.
func (us *UserStore) load() error {
	var list []*User
	err := us.store.ReadJSON(usersFile, &list)
	if errors.Is(err, os.ErrNotExist) {
		return nil // first run, no users yet
	}
	if err != nil {
		return fmt.Errorf("load users: %w", err)
	}

	for _, u := range list {
		us.users[u.Username] = u
	}
	return nil
}
