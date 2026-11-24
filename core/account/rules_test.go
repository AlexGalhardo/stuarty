package account

import (
	"slices"
	"strings"
	"testing"
)

func reasons(ps []Problem) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Field+":"+p.Reason)
	}

	return out
}

func TestUsernameBoundaries(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"three letters", "abc", ReasonTooShort},
		{"four letters", "abcd", ""},
		{"sixteen letters", strings.Repeat("a", 16), ""},
		{"seventeen letters", strings.Repeat("a", 17), ReasonTooLong},
		{"empty", "", ReasonTooShort},
		{"an uppercase letter", "abCd", ReasonNotLowercaseLetters},
		{"a digit", "abc1", ReasonNotLowercaseLetters},
		{"a dash", "ab-cd", ReasonNotLowercaseLetters},
		{"an accented letter", "abcé", ReasonNotLowercaseLetters},
		{"a space", "ab cd", ReasonNotLowercaseLetters},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Username(c.in)
			if c.want == "" {
				if got != nil {
					t.Fatalf("Username(%q) = %v, want none", c.in, got)
				}

				return
			}

			if len(got) != 1 || got[0].Reason != c.want || got[0].Field != FieldUsername {
				t.Fatalf("Username(%q) = %v, want %s", c.in, got, c.want)
			}
		})
	}
}

func TestEmailBoundariesAndFormat(t *testing.T) {
	pad := func(n int) string { return strings.Repeat("a", n) }

	cases := []struct {
		name, in, want string
	}{
		{"eleven characters", "ab@cde.fgh", ReasonTooShort},
		{"twelve characters", "abc@defg.com", ""},
		{"forty-eight characters", pad(48-len("@x.com")) + "@x.com", ""},
		{"forty-nine characters", pad(49-len("@x.com")) + "@x.com", ReasonTooLong},
		{"no at", "abcdefgh.com.br", ReasonInvalidFormat},
		{"two ats", "abc@def@ghi.com", ReasonInvalidFormat},
		{"no local part", "@abcdefgh.com", ReasonInvalidFormat},
		{"no dot in the domain", "abcdef@ghijklmn", ReasonInvalidFormat},
		{"one-letter top level", "abcdefgh@ijk.c", ReasonInvalidFormat},
		{"digits in the top level", "abcdefgh@ijk.c0m", ReasonInvalidFormat},
		{"two-letter top level", "abcdefgh@ijk.co", ""},
		{"empty label", "abcdefgh@ijk..com", ReasonInvalidFormat},
		{"leading dot in the local part", ".abcdefgh@ijk.com", ReasonInvalidFormat},
		{"a space", "abcd efgh@ijk.com", ReasonInvalidFormat},
		{"a plus tag", "abcd+tag@ijk.com", ""},
		{"a label that starts with a dash", "abcdefgh@-ijk.com", ReasonInvalidFormat},
		{"a non-ASCII local part", "abçdefgh@ijk.com", ReasonInvalidFormat},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Email(c.in)
			if c.want == "" {
				if got != nil {
					t.Fatalf("Email(%q) = %v, want none", c.in, got)
				}

				return
			}

			if len(got) != 1 || got[0].Reason != c.want || got[0].Field != FieldEmail {
				t.Fatalf("Email(%q) = %v, want %s", c.in, got, c.want)
			}
		})
	}
}

func TestPasswordBoundariesAndClasses(t *testing.T) {
	cases := []struct {
		name, in string
		want     []string
	}{
		{"seven characters", "Abcde1!", []string{"too_short"}},
		{"eight characters", "Abcdef1!", nil},
		{"thirty-two characters", "Aa1!" + strings.Repeat("x", 28), nil},
		{"thirty-three characters", "Aa1!" + strings.Repeat("x", 29), []string{"too_long"}},
		{"no uppercase", "abcdef1!", []string{"no_uppercase"}},
		{"no lowercase", "ABCDEF1!", []string{"no_lowercase"}},
		{"no digit", "Abcdefg!", []string{"no_digit"}},
		{"no special", "Abcdefg1", []string{"no_special"}},
		{"only letters", "Abcdefgh", []string{"no_digit", "no_special"}},
		{"a space", "Abc def1!", []string{"has_space_or_control"}},
		{"a tab", "Abcdef1!\t", []string{"has_space_or_control"}},
		{"a NUL", "Abcdef1!\x00", []string{"has_space_or_control"}},
		{"an accented lowercase letter counts as a lowercase", "ABCDEé1!", nil},
		{"a symbol outside ASCII is special", "Abcdef1€", nil},
		{"counted in characters, not bytes", "Aa1!éééé", nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got []string

			for _, p := range Password(c.in) {
				if p.Field != FieldPassword {
					t.Fatalf("field %q", p.Field)
				}

				got = append(got, p.Reason)
			}

			if !slices.Equal(got, c.want) {
				t.Fatalf("Password(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestConfirmation(t *testing.T) {
	if Confirmation("Abcdef1!", "Abcdef1!") != nil {
		t.Fatal("the same password was refused")
	}

	got := Confirmation("Abcdef1!", "Abcdef1?")
	if len(got) != 1 || got[0] != (Problem{FieldConfirmation, ReasonMismatch}) {
		t.Fatalf("got %v", got)
	}
}

func TestValidateReportsEveryField(t *testing.T) {
	got := reasons(Validate(Input{Username: "AB", Email: "x", Password: "abc", Confirmation: "abd"}))
	want := []string{
		"username:too_short", "email:too_short", "password:too_short", "confirmation:mismatch",
	}

	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	good := Input{Username: "stuart", Email: "stuart@example.com", Password: "Abcdef1!", Confirmation: "Abcdef1!"}
	if p := Validate(good); p != nil {
		t.Fatalf("a good input was refused: %v", p)
	}
}

func TestProblemNeverHoldsTheValue(t *testing.T) {
	secret := "Sup3r-Secret!"

	for _, p := range Validate(Input{Username: "a", Email: secret, Password: secret, Confirmation: "x"}) {
		if strings.Contains(p.Field+p.Reason, secret) {
			t.Fatalf("a problem holds the value: %v", p)
		}
	}
}

func TestIdleMinutes(t *testing.T) {
	for _, n := range []int{0, 1, 1440} {
		if IdleMinutes(n) != nil {
			t.Errorf("%d was refused", n)
		}
	}

	for _, n := range []int{-1, 1441} {
		if IdleMinutes(n) == nil {
			t.Errorf("%d was accepted", n)
		}
	}
}

// VV-380: the same text typed on another system arrives in another Unicode form. Both are one password: the
// count of characters, the classes and the confirmation see the composed form.
func TestPasswordIsComparedInItsComposedForm(t *testing.T) {
	const (
		composed   = "Abcdeé1!" // é as one character
		decomposed = "Abcdeé" + "1!"
	)

	if Normalize(decomposed) != composed {
		t.Fatalf("Normalize(%q) = %q, want %q", decomposed, Normalize(decomposed), composed)
	}

	if got := Password(decomposed); got != nil {
		t.Errorf("the decomposed form of an 8-character password = %v, want none", got)
	}

	// Seven composed characters are seven, in either form: the decomposed one is nine code points.
	short := "Abcdé" + "1!"
	if got := reasons(Password(short)); !slices.Equal(got, []string{"password:too_short"}) {
		t.Errorf("seven characters typed decomposed = %v, want too_short", got)
	}

	if got := Confirmation(composed, decomposed); got != nil {
		t.Errorf("the two forms of one password do not match: %v", got)
	}

	if got := Validate(Input{
		Username: "stuart", Email: "stuart@example.com", Password: composed, Confirmation: decomposed,
	}); got != nil {
		t.Errorf("Validate = %v, want none", got)
	}
}
