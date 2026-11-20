package ui

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"

	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// Every key of every owner and variant, and every command, passes one check.
func TestEveryKeyInUseIsCheckedAndClean(t *testing.T) {
	r := keyRegistry(keys.DefaultKeys())
	if len(r.Owners) < 15 || len(r.Commands) == 0 {
		t.Fatalf("the registry is nearly empty: %d owners, %d commands", len(r.Owners), len(r.Commands))
	}
	for _, c := range keys.Validate(r) {
		t.Error(c)
	}
}

// The check reads the real tables: `u` bound twice in the main table is named.
func TestValidateNamesAKeyBoundTwiceInTheRealMainTable(t *testing.T) {
	r := keyRegistry(keys.DefaultKeys())
	extra := []keys.BindingEntry{
		{ID: keys.BindingHelp, Binding: key.NewBinding(key.WithKeys("u"))},
		{ID: keys.BindingQuit, Binding: key.NewBinding(key.WithKeys("u"))},
	}
	r.Owners[keys.OwnerMain] = append(r.Owners[keys.OwnerMain], append(append([]keys.BindingEntry(nil), r.Owners[keys.OwnerMain][0]...), extra...))
	for _, c := range keys.Validate(r) {
		if c.Owner == "main" && c.Key == "u" {
			return
		}
	}
	t.Error("no clash named main u")
}
