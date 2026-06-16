package audit

import "testing"

func TestAuditKeyOrdering(t *testing.T) {
	// Keys for later timestamps must sort after earlier ones.
	k1 := auditKey(1000, 1)
	k2 := auditKey(2000, 1)
	k3 := auditKey(1000, 2)
	if string(k1) >= string(k2) {
		t.Errorf("k1 should sort before k2")
	}
	if string(k1) >= string(k3) {
		t.Errorf("same ts — k1 should sort before k3 by seq")
	}
}
