package ui

import (
	"fmt"

	"math"

	"github.com/gizak/termui"
	"github.com/gizak/termui/extra"
	"k8s.io/client-go/pkg/api/v1"
)

type Screen struct {
	tabs   []extra.Tab
	pods   *podsUI
	podsCh <-chan []v1.Pod
}

func New() *Screen {
	return &Screen{}
}

func (s *Screen) Init() error {
	err := termui.Init()
	if err != nil {
		return err
	}

	s.pods = newPodsUI()
	s.pods.layout()

	// setup event handles
	termui.Handle("/sys/kbd/p", func(termui.Event) {
		termui.Clear()
		s.pods.show()
	})

	termui.Handle("/sys/kbd/q", func(termui.Event) {
		termui.StopLoop()
	})

	termui.Handle("/sys/kbd/c", func(termui.Event) {
		s.pods.hide()
		termui.Clear()

		sinps := (func() []float64 {
			n := 220
			ps := make([]float64, n)
			for i := range ps {
				ps[i] = 1 + math.Sin(float64(i)/5)
			}
			return ps
		})()

		lc0 := termui.NewLineChart()
		lc0.BorderLabel = "braille-mode Line Chart"
		lc0.Data = sinps
		lc0.Width = 50
		lc0.Height = 12
		lc0.X = 0
		lc0.Y = 0
		lc0.AxesColor = termui.ColorWhite
		lc0.LineColor = termui.ColorGreen | termui.AttrBold

		termui.Render(lc0)

	})

	termui.Handle("/sys/wnd/resize", func(e termui.Event) {
		//termui.Clear()
		//termui.Render(s.pods.buffer())
	})

	return nil
}

func (s *Screen) RenderPods(pods <-chan []v1.Pod) {
	s.podsCh = pods
	s.pods.visible = true
	go func() {
		header := []string{
			"Name",
			"Namespace",
			"Status",
			"IP",
			"Node",
		}
		for pods := range s.podsCh {
			if !s.pods.visible {
				continue
			}
			var podgrid [][]string
			podgrid = append(podgrid, header)
			for _, pod := range pods {
				podgrid = append(podgrid, []string{
					pod.GetName(),
					pod.GetNamespace(),
					string(pod.Status.Phase),
					pod.Status.PodIP,
					pod.Spec.NodeName,
				})
			}
			s.pods.update(podgrid)
		}
	}()
}

// Open starts GUI loop
func (s *Screen) Open() {
	termui.Loop()
}

// Close closes GUI loop
func (s *Screen) Close() {
	termui.Close()
}

func newTab(label string) *extra.Tab {
	return extra.NewTab(fmt.Sprintf("  %s  ", label))
}
