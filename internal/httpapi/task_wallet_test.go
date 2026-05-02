package httpapi

import "testing"

func TestSelectWalletServiceForGenerationPrefersSDFor1KAndFallsBackToHD(t *testing.T) {
	service, err := selectWalletServiceForGeneration([]walletEntitlement{
		{ServiceCode: serviceCodeImage2SD, Remaining: 3, MaxConcurrent: 6},
		{ServiceCode: serviceCodeImage2HD, Remaining: 9, MaxConcurrent: 6},
	}, "1K")
	if err != nil {
		t.Fatalf("select service: %v", err)
	}
	if service.ServiceCode != serviceCodeImage2SD {
		t.Fatalf("service = %q", service.ServiceCode)
	}

	service, err = selectWalletServiceForGeneration([]walletEntitlement{
		{ServiceCode: serviceCodeImage2SD, Remaining: 0, MaxConcurrent: 6},
		{ServiceCode: serviceCodeImage2HD, Remaining: 9, MaxConcurrent: 6},
	}, "1K")
	if err != nil {
		t.Fatalf("select fallback service: %v", err)
	}
	if service.ServiceCode != serviceCodeImage2HD {
		t.Fatalf("fallback service = %q", service.ServiceCode)
	}
}

func TestSelectWalletServiceForGenerationRequiresHDFor2KAnd4K(t *testing.T) {
	_, err := selectWalletServiceForGeneration([]walletEntitlement{
		{ServiceCode: serviceCodeImage2SD, Remaining: 3, MaxConcurrent: 6},
	}, "2K")
	if err == nil {
		t.Fatalf("expected 2K without HD to fail")
	}

	for _, tier := range []string{"2K", "4K"} {
		service, err := selectWalletServiceForGeneration([]walletEntitlement{
			{ServiceCode: serviceCodeImage2SD, Remaining: 3, MaxConcurrent: 6},
			{ServiceCode: serviceCodeImage2HD, Remaining: 1, MaxConcurrent: 6},
		}, tier)
		if err != nil {
			t.Fatalf("select %s service: %v", tier, err)
		}
		if service.ServiceCode != serviceCodeImage2HD {
			t.Fatalf("%s service = %q", tier, service.ServiceCode)
		}
	}
}
