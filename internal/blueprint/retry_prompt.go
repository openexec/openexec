package blueprint

import "strings"

// CorrectionRequest gives an executor the task and observed failure while
// leaving the choice of repair to it, within the original authority.
func CorrectionRequest(task, failure, verification string) string {
	prompt := "The previous run failed.\n\nOriginal task:\n" + task +
		"\n\nError / failure evidence:\n" + failure
	if strings.TrimSpace(verification) != "" {
		prompt += "\n\nVerification check:\n" + verification
	}
	return prompt + "\n\nCould you fix this? Use your judgment to investigate and correct the cause, " +
		"preserve completed work, and verify the result. Stay within the original task's scope and authority."
}
