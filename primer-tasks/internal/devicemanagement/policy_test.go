package devicemanagement

import "testing"

const (
	studentSigner = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tvSigner      = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestValidatePolicyRequiresPinnedStudent(t *testing.T) {
	if err := ValidatePolicy(Policy{}); err == nil {
		t.Fatal("empty policy accepted")
	}
	if err := ValidatePolicy(Policy{ApprovedApps: []ApprovedApp{{PackageName: "com.example.other", SignerSHA256: studentSigner}}}); err == nil {
		t.Fatal("policy without Student accepted")
	}
	if err := ValidatePolicy(Policy{ApprovedApps: []ApprovedApp{{PackageName: StudentPackageName, SignerSHA256: "deadbeef"}}}); err == nil {
		t.Fatal("short signer accepted")
	}
	if err := ValidatePolicy(Policy{ApprovedApps: []ApprovedApp{{PackageName: "Com.Aleksclark.Primer.Student", SignerSHA256: studentSigner, Required: true}}}); err == nil {
		t.Fatal("case-folded Student package accepted")
	}
	if err := ValidatePolicy(Policy{ApprovedApps: []ApprovedApp{
		{PackageName: StudentPackageName, SignerSHA256: studentSigner, Required: true},
		{PackageName: StudentPackageName, SignerSHA256: tvSigner, Required: true},
	}}); err == nil {
		t.Fatal("conflicting Student signer accepted")
	}
	if err := ValidatePolicy(Policy{ApprovedApps: []ApprovedApp{{PackageName: StudentPackageName, SignerSHA256: studentSigner, Required: true}}, UserRestrictions: []UserRestriction{"remote_wipe"}}); err == nil {
		t.Fatal("unknown restriction accepted")
	}
	if err := ValidatePolicy(Policy{ApprovedApps: []ApprovedApp{{PackageName: StudentPackageName, SignerSHA256: studentSigner, Required: true}}, LockTask: LockTaskPolicy{Enabled: true, Packages: []string{"com.example.other"}}}); err == nil {
		t.Fatal("lock-task without Student accepted")
	}
	if err := ValidatePolicy(Policy{ApprovedApps: []ApprovedApp{
		{PackageName: StudentPackageName, SignerSHA256: studentSigner, Required: true},
		{PackageName: "com.aleksclark.primer.tv", SignerSHA256: tvSigner},
	}}); err != nil {
		t.Fatal(err)
	}
}
