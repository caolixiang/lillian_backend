package httpapi

import (
	"strings"
	"testing"
)

func TestAdminLicenseListQueryFiltersByServiceAndRedemptionState(t *testing.T) {
	query, args := adminLicenseListQuery(adminLicenseListFilters{
		SearchHash:  "hash:license",
		ServiceCode: serviceCodeImage2HD,
		Redeemed:    "used",
		Limit:       21,
		Offset:      40,
	})

	for _, want := range []string{
		"l.key_hash = $1",
		"l.service_code = $2",
		"l.redeemed_wallet_id IS NOT NULL",
		"LIMIT $3 OFFSET $4",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("query missing %q:\n%s", want, query)
		}
	}
	if len(args) != 4 || args[0] != "hash:license" || args[1] != serviceCodeImage2HD || args[2] != 21 || args[3] != 40 {
		t.Fatalf("args = %#v", args)
	}
}

func TestAdminLicenseListQueryFiltersUnusedWithoutSearch(t *testing.T) {
	query, args := adminLicenseListQuery(adminLicenseListFilters{
		ServiceCode: serviceCodeImage2SD,
		Redeemed:    "unused",
		Limit:       51,
		Offset:      0,
	})

	for _, want := range []string{
		"l.status <> 'deleted'",
		"l.service_code = $1",
		"l.redeemed_wallet_id IS NULL",
		"LIMIT $2 OFFSET $3",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("query missing %q:\n%s", want, query)
		}
	}
	if strings.Contains(query, "l.key_hash") {
		t.Fatalf("query unexpectedly includes key hash filter:\n%s", query)
	}
	if len(args) != 3 || args[0] != serviceCodeImage2SD || args[1] != 51 || args[2] != 0 {
		t.Fatalf("args = %#v", args)
	}
}
