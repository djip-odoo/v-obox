package printer

import (
	"testing"
)

func TestSanitizeDeviceID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "contains null bytes and control chars", input: "MFG:EPSON\x00;MDL:\x01TM-T20II\x00;\x1f", expected: "MFG:EPSON;MDL:TM-T20II;"},
		{name: "unicode printables preserved", input: "MFG:ÜberPrinter;", expected: "MFG:ÜberPrinter;"},
		{name: "plain ascii", input: "MFG:EPSON;MDL:TM-T20II;", expected: "MFG:EPSON;MDL:TM-T20II;"},
		{name: "empty string", input: "", expected: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeDeviceID(tc.input)
			if got != tc.expected {
				t.Errorf("sanitizeDeviceID(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestNormalizeKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"CMD", "CMD"},
		{"command set", "CMD"},
		{"COMMANDSET", "CMD"},
		{"command", "CMD"},
		{"commands", "CMD"},
		{"MFG", "MFG"},
		{"manufacturer", "MFG"},
		{"MDL", "MDL"},
		{"model", "MDL"},
		{"CLS", "CLS"},
		{"class", "CLS"},
		{"custom_key", "CUSTOM_KEY"},
		{"  mfg  ", "MFG"},
	}

	for _, tc := range tests {
		got := normalizeKey(tc.input)
		if got != tc.expected {
			t.Errorf("normalizeKey(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestParseDeviceID(t *testing.T) {
	raw := "MFG:EPSON;CMD:ESC/POS;MDL:TM-T20II;CLS:PRINTER;DESCRIPTION:Receipt Printer;"
	got := parseDeviceID(raw)

	if got["MFG"] != "EPSON" {
		t.Errorf("Expected MFG=EPSON, got: %s", got["MFG"])
	}
	if got["CMD"] != "ESC/POS" {
		t.Errorf("Expected CMD=ESC/POS, got: %s", got["CMD"])
	}
	if got["MDL"] != "TM-T20II" {
		t.Errorf("Expected MDL=TM-T20II, got: %s", got["MDL"])
	}
	if got["CLS"] != "PRINTER" {
		t.Errorf("Expected CLS=PRINTER, got: %s", got["CLS"])
	}
	if got["DESCRIPTION"] != "Receipt Printer" {
		t.Errorf("Expected DESCRIPTION='Receipt Printer', got: %s", got["DESCRIPTION"])
	}
}

func TestParseDeviceID_DuplicateKeys(t *testing.T) {
	// Aliases map "COMMAND" and "CMD" both to "CMD" -> should be concatenated with comma
	raw := "CMD:ESC/POS;COMMAND:STAR;"
	got := parseDeviceID(raw)

	if got["CMD"] != "ESC/POS,STAR" {
		t.Errorf("Expected CMD='ESC/POS,STAR', got: %s", got["CMD"])
	}

	// Repeated identical value should not be duplicated
	rawSame := "CMD:ESC/POS;COMMAND:ESC/POS;"
	gotSame := parseDeviceID(rawSame)
	if gotSame["CMD"] != "ESC/POS" {
		t.Errorf("Expected CMD='ESC/POS', got: %s", gotSame["CMD"])
	}
}

func TestParseDeviceID_MalformedAndEdgeCases(t *testing.T) {
	// Empty segments, missing colon, trailing semicolons
	raw := ";;;NO_COLON;EMPTY_VAL:;:EMPTY_KEY;  ;MFG:ACME;"
	got := parseDeviceID(raw)

	if len(got) != 1 {
		t.Errorf("Expected exactly 1 valid key-value pair, got %d: %v", len(got), got)
	}
	if got["MFG"] != "ACME" {
		t.Errorf("Expected MFG=ACME, got: %s", got["MFG"])
	}
}
