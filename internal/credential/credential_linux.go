//go:build linux

package credential

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
)

// Linux credential storage is the freedesktop.org Secret Service API over the
// session bus. GNOME Keyring, KWallet and KeePassXC all implement it, so one
// client covers the desktops Overgent expects to run on without shelling out
// to secret-tool or libsecret (no CGO, per AGENTS.md).
//
// Items are keyed exactly the way the macOS implementation keys them: the
// serviceName constant as the "service" attribute and the caller's account as
// the "account" attribute. An item written by one platform is therefore
// findable by the same account string on the other, which matters because the
// account names are minted by shared code (device IDs, backend instance
// names), not per platform.
const (
	secretServiceDest   = "org.freedesktop.secrets"
	secretServicePath   = dbus.ObjectPath("/org/freedesktop/secrets")
	defaultCollection   = dbus.ObjectPath("/org/freedesktop/secrets/aliases/default")
	serviceInterface    = "org.freedesktop.Secret.Service"
	collectionInterface = "org.freedesktop.Secret.Collection"
	itemInterface       = "org.freedesktop.Secret.Item"
	sessionInterface    = "org.freedesktop.Secret.Session"
	promptInterface     = "org.freedesktop.Secret.Prompt"

	// noObject is the path a Secret Service method returns for "no prompt is
	// needed" and for "nothing was created".
	noObject = dbus.ObjectPath("/")

	// promptTimeout bounds an interactive prompt when the caller's context has
	// no deadline of its own. A keyring unlock can legitimately wait on a
	// member typing a password, so it is generous; it exists only so a
	// long-lived service cannot be wedged forever by a dialog nobody is
	// looking at.
	promptTimeout = 2 * time.Minute

	// contentType is the Secret Service content type for the UTF-8 secrets
	// Overgent stores (hex device tokens, backend secrets, provider API keys).
	contentType = "text/plain; charset=utf8"
)

// dbusSecret mirrors the Secret Service "(oayays)" secret structure.
type dbusSecret struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

// secretSession is one private connection to the session bus plus one open
// Secret Service session. It is opened per operation and closed at the end:
// credential access is rare and never on a hot path, and a per-operation
// connection means a keyring daemon restart cannot leave a stale handle
// behind.
type secretSession struct {
	conn *dbus.Conn
	path dbus.ObjectPath
}

// The "plain" session algorithm sends the secret unencrypted over the D-Bus
// connection, and that is acceptable here.
//
// The session bus is a Unix domain socket owned by this same user, with the
// same 0600-equivalent access as the process's own memory; a peer that can
// read it can already ptrace this process. The alternative,
// dh-ietf1024-sha256-aes128-cbc-pkcs7, protects the value from the bus daemon
// itself, which is only a distinct trust domain when the bus is proxied over a
// socket others can reach (a Flatpak portal, or a forwarded TCP session bus).
// Overgent runs unsandboxed as the member's own user, so the extra Diffie-
// Hellman handshake would add an untested crypto path without moving a trust
// boundary. If Overgent is ever shipped inside a Flatpak or Snap, revisit this
// and implement the encrypted algorithm before doing so.
const sessionAlgorithm = "plain"

// OPEN OWNER DECISION - headless Linux.
//
// A Linux host with no Secret Service provider on the session bus (an SSH box,
// a container, WSL without a keyring daemon) cannot store a credential at all,
// so it cannot join a team Project and cannot run a local backend. Every
// failure below is an *UnavailableError saying exactly that, and the member's
// only route today is to install a keyring.
//
// Whether that is the shipped answer is not this package's call. The options,
// for whoever decides:
//
//  1. Keep it as is. Headless Linux is unsupported for credential-holding
//     operations. Honest, no new attack surface, and it locks out the
//     server-side and devcontainer use of Overgent entirely.
//  2. Ship gnome-keyring-daemon guidance plus a documented dbus-run-session
//     wrapper. No code, no new secret handling; still needs a daemon present.
//  3. Add a passphrase-derived file store for headless hosts only, unlocked
//     once per service start. This is a genuine weakening: the secret becomes
//     recoverable from disk plus a passphrase, which is the property the OS
//     store exists to remove, and it needs an ADR that supersedes the
//     "no plaintext fallback" rule with an explicit scope.
//
// Recommendation: (2) now, and (1) as the stated support boundary. Do not add
// (3) without an ADR, and if it is added, gate it on an explicit member opt-in
// per host rather than on "no keyring was found", so it can never be reached
// by silent degradation on a desktop whose keyring merely failed to start.
func unavailable(reason, remedy string, cause error) error {
	return &UnavailableError{Platform: "linux", Reason: reason, Remedy: remedy, Cause: cause}
}

func openSecretSession(ctx context.Context) (*secretSession, error) {
	conn, err := dbus.SessionBusPrivate()
	if err != nil {
		return nil, unavailable(
			"there is no D-Bus session bus for this login",
			"run Overgent inside a desktop session, or start one with dbus-run-session and set DBUS_SESSION_BUS_ADDRESS",
			err,
		)
	}
	if err := conn.Auth(nil); err != nil {
		_ = conn.Close()
		return nil, unavailable(
			"the D-Bus session bus rejected authentication",
			"check that $DBUS_SESSION_BUS_ADDRESS points at this user's own bus",
			err,
		)
	}
	if err := conn.Hello(); err != nil {
		_ = conn.Close()
		return nil, unavailable(
			"the D-Bus session bus handshake failed",
			"check that $DBUS_SESSION_BUS_ADDRESS points at this user's own bus",
			err,
		)
	}
	var (
		output dbus.Variant
		path   dbus.ObjectPath
	)
	err = conn.Object(secretServiceDest, secretServicePath).
		CallWithContext(ctx, serviceInterface+".OpenSession", 0, sessionAlgorithm, dbus.MakeVariant("")).
		Store(&output, &path)
	if err != nil {
		_ = conn.Close()
		return nil, unavailable(
			"no org.freedesktop.secrets provider is running on the session bus",
			"install and start a keyring that implements the Secret Service API (gnome-keyring-daemon, kwalletd, or KeePassXC with Secret Service integration enabled)",
			err,
		)
	}
	return &secretSession{conn: conn, path: path}, nil
}

func (s *secretSession) close() {
	if s.path != "" && s.path != noObject {
		_ = s.conn.Object(secretServiceDest, s.path).Call(sessionInterface+".Close", 0).Err
	}
	_ = s.conn.Close()
}

func (s *secretSession) service() dbus.BusObject {
	return s.conn.Object(secretServiceDest, secretServicePath)
}

// awaitPrompt drives the Prompt object a Secret Service method handed back and
// waits for its Completed signal. The keyring shows a dialog here (unlock the
// login keyring, allow this application), so the member is expected to act.
func (s *secretSession) awaitPrompt(ctx context.Context, prompt dbus.ObjectPath) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, promptTimeout)
		defer cancel()
	}
	match := []dbus.MatchOption{
		dbus.WithMatchObjectPath(prompt),
		dbus.WithMatchInterface(promptInterface),
		dbus.WithMatchMember("Completed"),
	}
	// Subscribe before calling Prompt: the keyring may complete a prompt it
	// does not need to show before the call even returns.
	signals := make(chan *dbus.Signal, 4)
	s.conn.Signal(signals)
	defer s.conn.RemoveSignal(signals)
	if err := s.conn.AddMatchSignalContext(ctx, match...); err != nil {
		return fmt.Errorf("watch the Secret Service prompt: %w", err)
	}
	defer func() { _ = s.conn.RemoveMatchSignal(match...) }()

	promptObject := s.conn.Object(secretServiceDest, prompt)
	if call := promptObject.CallWithContext(ctx, promptInterface+".Prompt", 0, ""); call.Err != nil {
		return fmt.Errorf("open the Secret Service prompt: %w", call.Err)
	}
	for {
		select {
		case <-ctx.Done():
			// Leaving the dialog on screen after we have stopped listening
			// would be worse than a failed operation.
			_ = promptObject.Call(promptInterface+".Dismiss", 0).Err
			return fmt.Errorf("wait for the Secret Service prompt: %w", ctx.Err())
		case signal := <-signals:
			if signal == nil || signal.Path != prompt || signal.Name != promptInterface+".Completed" {
				continue
			}
			if len(signal.Body) != 2 {
				return errors.New("the Secret Service prompt returned a malformed result")
			}
			if dismissed, _ := signal.Body[0].(bool); dismissed {
				return errors.New("the Secret Service prompt was dismissed; the keyring stayed locked")
			}
			return nil
		}
	}
}

// unlock unlocks the given objects if the Secret Service reports them locked.
// It is safe to call on objects that are already unlocked.
func (s *secretSession) unlock(ctx context.Context, paths ...dbus.ObjectPath) error {
	if len(paths) == 0 {
		return nil
	}
	var (
		unlocked []dbus.ObjectPath
		prompt   dbus.ObjectPath
	)
	if err := s.service().CallWithContext(ctx, serviceInterface+".Unlock", 0, paths).Store(&unlocked, &prompt); err != nil {
		return fmt.Errorf("unlock the keyring: %w", err)
	}
	if prompt == noObject || prompt == "" {
		return nil
	}
	return s.awaitPrompt(ctx, prompt)
}

// unlockDefaultCollection unlocks the default collection when it is locked. A
// freshly booted session commonly has a locked login keyring, and CreateItem
// against a locked collection fails rather than prompting on its own.
func (s *secretSession) unlockDefaultCollection(ctx context.Context) error {
	collection := s.conn.Object(secretServiceDest, defaultCollection)
	property, err := collection.GetProperty(collectionInterface + ".Locked")
	if err != nil {
		return unavailable(
			"the Secret Service has no default collection",
			"create a default keyring (in GNOME: Passwords and Keys, then a new keyring named Login) and try again",
			err,
		)
	}
	locked, ok := property.Value().(bool)
	if !ok {
		return errors.New("the Secret Service reported a non-boolean Locked property")
	}
	if !locked {
		return nil
	}
	return s.unlock(ctx, defaultCollection)
}

func attributes(account string) map[string]string {
	return map[string]string{"service": serviceName, "account": account}
}

// searchItems returns every item matching the account, unlocking any that come
// back locked so the caller can read or delete them.
func (s *secretSession) searchItems(ctx context.Context, account string) ([]dbus.ObjectPath, error) {
	var unlocked, locked []dbus.ObjectPath
	err := s.service().
		CallWithContext(ctx, serviceInterface+".SearchItems", 0, attributes(account)).
		Store(&unlocked, &locked)
	if err != nil {
		return nil, fmt.Errorf("search the Secret Service for the credential: %w", err)
	}
	if len(locked) > 0 {
		if err := s.unlock(ctx, locked...); err != nil {
			return nil, err
		}
		unlocked = append(unlocked, locked...)
	}
	return unlocked, nil
}

func put(ctx context.Context, account, secret string) error {
	if err := validatePut(account, secret); err != nil {
		return err
	}
	session, err := openSecretSession(ctx)
	if err != nil {
		return err
	}
	defer session.close()
	if err := session.unlockDefaultCollection(ctx); err != nil {
		return err
	}

	value := []byte(secret)
	defer zeroBytes(value)
	properties := map[string]dbus.Variant{
		itemInterface + ".Label":      dbus.MakeVariant(serviceName + " (" + account + ")"),
		itemInterface + ".Attributes": dbus.MakeVariant(attributes(account)),
	}
	item := dbusSecret{
		Session:     session.path,
		Parameters:  []byte{},
		Value:       value,
		ContentType: contentType,
	}
	var created, prompt dbus.ObjectPath
	// replace=true makes this an upsert, matching the macOS -U flag: storing
	// a rotated device credential must not leave the old one findable.
	err = session.conn.Object(secretServiceDest, defaultCollection).
		CallWithContext(ctx, collectionInterface+".CreateItem", 0, properties, item, true).
		Store(&created, &prompt)
	if err != nil {
		return fmt.Errorf("store the credential in the Secret Service default collection: %w", err)
	}
	if prompt != noObject && prompt != "" {
		if err := session.awaitPrompt(ctx, prompt); err != nil {
			return err
		}
		return nil
	}
	if created == noObject || created == "" {
		return errors.New("the Secret Service did not create the credential item")
	}
	return nil
}

func get(ctx context.Context, account string) (string, error) {
	if err := validateAccount(account); err != nil {
		return "", err
	}
	session, err := openSecretSession(ctx)
	if err != nil {
		return "", err
	}
	defer session.close()

	items, err := session.searchItems(ctx, account)
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		return "", fmt.Errorf("read the credential from the Secret Service: %w", ErrNotFound)
	}
	var stored dbusSecret
	err = session.conn.Object(secretServiceDest, items[0]).
		CallWithContext(ctx, itemInterface+".GetSecret", 0, session.path).
		Store(&stored)
	if err != nil {
		return "", fmt.Errorf("read the credential from the Secret Service: %w", err)
	}
	defer zeroBytes(stored.Value)
	if len(stored.Value) == 0 {
		return "", errors.New("the Secret Service returned an empty credential")
	}
	return string(stored.Value), nil
}

func remove(ctx context.Context, account string) error {
	if err := validateAccount(account); err != nil {
		return err
	}
	session, err := openSecretSession(ctx)
	if err != nil {
		return err
	}
	defer session.close()

	items, err := session.searchItems(ctx, account)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("delete the credential from the Secret Service: %w", ErrNotFound)
	}
	// A rotation that raced with an older client can leave duplicates under
	// one account; delete every match so a later Get cannot resurrect one.
	for _, item := range items {
		var prompt dbus.ObjectPath
		if err := session.conn.Object(secretServiceDest, item).
			CallWithContext(ctx, itemInterface+".Delete", 0).
			Store(&prompt); err != nil {
			return fmt.Errorf("delete the credential from the Secret Service: %w", err)
		}
		if prompt != noObject && prompt != "" {
			if err := session.awaitPrompt(ctx, prompt); err != nil {
				return err
			}
		}
	}
	return nil
}
