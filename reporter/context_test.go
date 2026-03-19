package reporter

import "testing"

func TestContextPrint(t *testing.T) {
	c := &testContext{}
	if i, err := c.print("test"); err != nil {
		t.Fatal(err)
	} else if i != 0 {
		t.Fatalf("expect 0 but got %d", i)
	}
	if i, err := c.printf("%s", "test"); err != nil {
		t.Fatal(err)
	} else if i != 0 {
		t.Fatalf("expect 0 but got %d", i)
	}
}
