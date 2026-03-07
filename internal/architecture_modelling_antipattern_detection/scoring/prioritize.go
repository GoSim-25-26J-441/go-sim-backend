package scoring

import (
	"sort"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
)

func PrioritizeDetections(dets []domain.Detection) []domain.Detection {
	sort.SliceStable(dets, func(i, j int) bool {
		si := ScoreDetection(dets[i])
		sj := ScoreDetection(dets[j])
		if si != sj {
			return si > sj
		}
		return dets[i].Severity > dets[j].Severity
	})
	return dets
}
