package devicemanagement

import "testing"

func TestValidatePolicyRequiresPinnedStudent(t *testing.T) {
	if err := ValidatePolicy(Policy{}); err == nil {
		t.Fatal("empty policy accepted")
	}
	if err := ValidatePolicy(Policy{ApprovedApps: []ApprovedApp{{PackageName: "com.example.other", SignerSHA256: "abc"}}}); err == nil {
		t.Fatal("policy without Student accepted")
	}
	if err := ValidatePolicy(Policy{ApprovedApps: []ApprovedApp{{PackageName: StudentPackageName, SignerSHA256: ""}}}); err == nil {
		t.Fatal("unsigned Student accepted")
	}
	if err := ValidatePolicy(Policy{ApprovedApps: []ApprovedApp{{PackageName: StudentPackageName, SignerSHA256: "deadbeef"}}}); err != nil {
		t.Fatal(err)
	}
}
