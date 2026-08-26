package domain

type Status string

const (
	StatusDraft         Status = "DRAFT"
	StatusBaselined     Status = "BASELINED"
	StatusDataChecked   Status = "DATA_CHECKED"
	StatusTriaged       Status = "TRIAGED"
	StatusReviewPending Status = "REVIEW_PENDING"
	StatusReviewed      Status = "REVIEWED"
	StatusDecided       Status = "DECIDED"
	StatusArchived      Status = "ARCHIVED"
)

func (s Status) String() string { return string(s) }
func (s Status) Terminal() bool { return s == StatusArchived }
func ValidStatus(s Status) bool {
	switch s {
	case StatusDraft, StatusBaselined, StatusDataChecked, StatusTriaged, StatusReviewPending, StatusReviewed, StatusDecided, StatusArchived:
		return true
	}
	return false
}
func CanTransition(from, to Status) bool {
	if from == StatusDraft && to == StatusBaselined {
		return true
	}
	if from == StatusBaselined && to == StatusDataChecked {
		return true
	}
	if from == StatusDataChecked && (to == StatusTriaged || to == StatusReviewPending) {
		return true
	}
	if from == StatusTriaged && to == StatusReviewPending {
		return true
	}
	if from == StatusReviewPending && (to == StatusTriaged || to == StatusReviewed) {
		return true
	}
	if from == StatusReviewed && to == StatusDecided {
		return true
	}
	if from == StatusDecided && to == StatusArchived {
		return true
	}
	return false
}
func StatusSequence() []Status {
	return []Status{StatusDraft, StatusBaselined, StatusDataChecked, StatusTriaged, StatusReviewPending, StatusReviewed, StatusDecided, StatusArchived}
}
