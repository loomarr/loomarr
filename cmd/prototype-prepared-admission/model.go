// PROTOTYPE — this state model answers one question for #1097:
// can capacity-N admit N-1 background publications, preserve one foreground reserve,
// and cancel only the least-urgent background work as foreground demand grows?
package main

import "slices"

type backgroundLease struct {
	ID      int
	Urgency int
}

type admissionState struct {
	Capacity   int
	Foreground int
	Background []backgroundLease
	Cancelled  []int
	NextID     int
}

func (s admissionState) idle() int {
	return s.Capacity - s.Foreground - len(s.Background)
}

func (s admissionState) addBackground(urgency int) admissionState {
	if s.Capacity < 2 || s.idle() <= 1 {
		return s
	}
	s.NextID++
	s.Background = append(slices.Clone(s.Background), backgroundLease{ID: s.NextID, Urgency: urgency})
	return s
}

func (s admissionState) addForeground() admissionState {
	if s.Capacity <= 0 || s.Foreground >= s.Capacity {
		return s
	}
	s.Foreground++
	if s.idle() >= 0 {
		return s
	}
	background := slices.Clone(s.Background)
	leastUrgent := 0
	for i := 1; i < len(background); i++ {
		if background[i].Urgency > background[leastUrgent].Urgency ||
			(background[i].Urgency == background[leastUrgent].Urgency && background[i].ID > background[leastUrgent].ID) {
			leastUrgent = i
		}
	}
	s.Cancelled = append(slices.Clone(s.Cancelled), background[leastUrgent].ID)
	s.Background = slices.Delete(background, leastUrgent, leastUrgent+1)
	return s
}

func (s admissionState) releaseForeground() admissionState {
	if s.Foreground > 0 {
		s.Foreground--
	}
	return s
}

func (s admissionState) releaseBackground() admissionState {
	if len(s.Background) == 0 {
		return s
	}
	s.Background = slices.Clone(s.Background[:len(s.Background)-1])
	return s
}
