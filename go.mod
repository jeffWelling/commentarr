module github.com/jeffWelling/commentarr

go 1.25.0

require github.com/jeffWelling/commentary-classifier v0.0.0-00010101000000-000000000000

// Local development: point at the sibling checkout until we publish a
// tagged version to GitHub.
replace github.com/jeffWelling/commentary-classifier => ../commentary-classifier
