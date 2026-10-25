package store

import (
	"os"
	"testing"
)

// C13: the reader reports both values with a presence marker and an ordered
// list of rejections, and holds nothing else.
func TestReadKeysBlock(t *testing.T) {
	path := prefPath(t)
	seedPreferences(t, path, "keys:\n  leader: \",\"\n  timeoutlen: 750\n")

	got := Keys()
	if !got.LeaderPresent || got.Leader != "," {
		t.Errorf("leader = %q present=%v, want \",\" present", got.Leader, got.LeaderPresent)
	}
	if !got.TimeoutlenPresent || got.TimeoutlenMS != 750 {
		t.Errorf("timeoutlen = %d present=%v, want 750 present", got.TimeoutlenMS, got.TimeoutlenPresent)
	}
	if len(got.Rejections) != 0 {
		t.Errorf("clean block produced rejections: %+v", got.Rejections)
	}
}

// C13, ordering: rejections come back in the order the file listed them.
func TestReadKeysBlockRejectionOrder(t *testing.T) {
	path := prefPath(t)
	seedPreferences(t, path, "keys:\n  timeoutlen: soon\n  leader: \",\"\n")

	got := Keys()
	if len(got.Rejections) != 1 {
		t.Fatalf("rejections = %+v, want exactly one", got.Rejections)
	}
	r := got.Rejections[0]
	if r.Field != "keys.timeoutlen" || r.Value != "soon" || r.Reason == "" {
		t.Errorf("rejection = %+v, want field/value/reason all filled", r)
	}
}

// C14: absent, or present and empty, both read as absent with no rejection.
func TestKeysLeaderAbsentOrEmpty(t *testing.T) {
	cases := map[string]string{
		"no keys block":   "location:\n  name: Berlin\n",
		"no leader entry": "keys:\n  timeoutlen: 750\n",
		"bare empty":      "keys:\n  leader:\n",
		"quoted empty":    "keys:\n  leader: \"\"\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := prefPath(t)
			seedPreferences(t, path, body)
			got := Keys()
			if got.LeaderPresent {
				t.Errorf("leader reported present as %q", got.Leader)
			}
			for _, r := range got.Rejections {
				if r.Field == "keys.leader" {
					t.Errorf("empty leader must not be a rejection, got %+v", r)
				}
			}
		})
	}
}

// C15: timeoutlen is a positive whole number of milliseconds; anything else
// reads as absent and records exactly one rejection.
func TestKeysTimeoutlenParsing(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		path := prefPath(t)
		seedPreferences(t, path, "keys:\n  timeoutlen: 1\n")
		got := Keys()
		if !got.TimeoutlenPresent || got.TimeoutlenMS != 1 {
			t.Errorf("got %d present=%v, want 1 present", got.TimeoutlenMS, got.TimeoutlenPresent)
		}
		if len(got.Rejections) != 0 {
			t.Errorf("unexpected rejections: %+v", got.Rejections)
		}
	})
	for _, bad := range []string{"0", "-1", "1.5", "soon", "1e3", "750ms"} {
		t.Run("rejects "+bad, func(t *testing.T) {
			path := prefPath(t)
			seedPreferences(t, path, "keys:\n  timeoutlen: "+bad+"\n")
			got := Keys()
			if got.TimeoutlenPresent {
				t.Errorf("%q reported present as %d", bad, got.TimeoutlenMS)
			}
			if len(got.Rejections) != 1 {
				t.Fatalf("%q produced rejections %+v, want exactly one", bad, got.Rejections)
			}
			if got.Rejections[0].Value != bad {
				t.Errorf("rejection value = %q, want %q", got.Rejections[0].Value, bad)
			}
		})
	}
}

// C16: one bad entry does not cost the block its other entries, nor the file
// its other blocks.
func TestKeysBlockKeepsValidEntries(t *testing.T) {
	path := prefPath(t)
	seedPreferences(t, path,
		"keys:\n"+
			"  timeoutlen: nonsense\n"+
			"  leader: \";\"\n"+
			"  future: whatever\n"+
			berlinBlock)

	got := Keys()
	if !got.LeaderPresent || got.Leader != ";" {
		t.Errorf("leader = %q present=%v, want \";\" present", got.Leader, got.LeaderPresent)
	}
	if len(got.Rejections) != 1 {
		t.Errorf("rejections = %+v, want only the timeoutlen one (an unknown key is not a mistake)", got.Rejections)
	}
	if _, ok := Location(); !ok {
		t.Error("the location: block stopped reading because the keys: block had a bad entry")
	}
}

// C17: no file, or a file that cannot be read, is silence — not an error.
func TestMissingPreferencesIsSilent(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		prefPath(t)
		got := Keys()
		if got.LeaderPresent || got.TimeoutlenPresent || len(got.Rejections) != 0 {
			t.Errorf("absent file reported %+v", got)
		}
	})
	t.Run("unreadable", func(t *testing.T) {
		requireUnprivileged(t)
		path := prefPath(t)
		seedPreferences(t, path, "keys:\n  leader: \";\"\n")
		if err := os.Chmod(path, 0o000); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
		got := Keys()
		if got.LeaderPresent || got.TimeoutlenPresent || len(got.Rejections) != 0 {
			t.Errorf("unreadable file reported %+v", got)
		}
	})
}

// The reader owns the FIRST keys: block, exactly as the writer does. A second
// one is somebody else's and is not merged into it — otherwise a value from a
// block no writer will touch could sit beside a rejection from the one it will.
func TestKeysReadsFirstBlockOnly(t *testing.T) {
	path := prefPath(t)
	seedPreferences(t, path,
		"keys:\n  timeoutlen: 750\n"+
			berlinBlock+
			"keys:\n  timeoutlen: soon\n  leader: \";\"\n")

	got := Keys()
	if !got.TimeoutlenPresent || got.TimeoutlenMS != 750 {
		t.Errorf("timeoutlen = %d present=%v, want 750 from the first block", got.TimeoutlenMS, got.TimeoutlenPresent)
	}
	if len(got.Rejections) != 0 {
		t.Errorf("a second block's bad entry was reported: %+v", got.Rejections)
	}
	if got.LeaderPresent {
		t.Errorf("a second block's leader was read as %q", got.Leader)
	}
}
