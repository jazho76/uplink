package tui

const (
	borderCells  = 2
	paddingCells = 2
	chromeCells  = borderCells + paddingCells

	hostBarRows = 1 + borderCells
	footerRows  = 2

	paneColumnPercent = 38
	minPaneColumnW    = 22
	minPreviewW       = 16
	minBodyH          = 9

	minWidth  = minPaneColumnW + minPreviewW
	minHeight = minBodyH + hostBarRows + footerRows
)

type layout struct {
	paneColumnOuterW int
	previewOuterW    int
	bodyOuterH       int
}

func computeLayout(width, height int) layout {
	l := layout{bodyOuterH: max(height-hostBarRows-footerRows, minBodyH)}

	l.paneColumnOuterW = max(width*paneColumnPercent/100, minPaneColumnW)
	l.previewOuterW = width - l.paneColumnOuterW
	if l.previewOuterW < minPreviewW {
		l.previewOuterW = minPreviewW
		l.paneColumnOuterW = width - l.previewOuterW
	}
	return l
}

func shares(total, buckets int) []int {
	if buckets < 1 {
		return nil
	}
	out := make([]int, buckets)
	for i := range out {
		out[i] = total / buckets
		if i < total%buckets {
			out[i]++
		}
	}
	return out
}
