//go:build !darwin

package mode

type quickSearchOutsideMonitor struct{}

func (q *quickSearchMode) startOutsideClickWatch()   {}
func (q *quickSearchMode) stopOutsideClickWatch()    {}
func (q *quickSearchMode) releaseOutsideClickWatch() {}
