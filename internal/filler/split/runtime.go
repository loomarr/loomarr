package split

// AssessmentSource is the exact retained source an assessment reads, and the file it reads it from.
type AssessmentSource struct {
	Source   SourceAsset
	FullPath string
}
