package querier

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gdamore/tcell/v2"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/util/teststorage"
	"github.com/rivo/tview"
)

const header = `                .__     
  ____  ______  |  |__  
_/ __ \ \____ \ |  |  \ 
\  ___/ |  |_> >|   Y  \
 \___  >|   __/ |___|  /
     \/ |__|         \/ 
`

func OpenUI(bucket string, files []FileItem, s3client s3.Client) {
	fmt.Println("start tview")
	screen, err := tcell.NewScreen()
	if err != nil {
		log.Fatalf("Failed to create screen: %v", err)
	}
	if err := screen.Init(); err != nil {
		log.Fatalf("Failed to init screen: %v", err)
	}
	defer screen.Fini()

	app := tview.NewApplication().SetScreen(screen)

	// Build table
	table := tview.NewTable()
	table.
		SetBorders(false).
		SetSelectable(true, false).
		SetBorder(true).
		SetTitle(fmt.Sprintf("  [orange]bucket: %s  ", bucket))

	headers := []string{" Name", "Size (bytes)", "Last Modified", "Downloaded"}
	for i, h := range headers {
		table.SetCell(0, i,
			tview.NewTableCell(fmt.Sprintf("[orange]%s", h)).
				SetSelectable(false).
				SetAlign(tview.AlignLeft))
	}

	for r, file := range files {
		status := "-"
		if len(file.Data) > 0 {
			status = "Downloaded"
		}
		table.SetCell(r+1, 0, tview.NewTableCell(fmt.Sprintf(" %s", file.Name)).SetExpansion(100))
		table.SetCell(r+1, 1, tview.NewTableCell(fmt.Sprintf("%d", file.Size)).SetExpansion(50))
		table.SetCell(r+1, 2, tview.NewTableCell(file.Date.Format(time.RFC3339)).SetExpansion(50))
		table.SetCell(r+1, 3, tview.NewTableCell(status).SetExpansion(50))
	}

	headerText := fmt.Sprintf("[white::b]Bucket: %s   Items: %d", bucket, len(files))
	footerText := "[::d]↑↓ Navigate   Enter View Details   Esc Quit"

	frame := tview.NewFrame(table).
		SetBorders(1, 1, 1, 1, 0, 0).
		AddText(headerText, true, tview.AlignLeft, tcell.ColorBlack).
		AddText("", true, tview.AlignCenter, tcell.ColorDefault).
		AddText(footerText, false, tview.AlignCenter, tcell.ColorGray)

	// Create pages layout
	pages := tview.NewPages().
		AddPage("main", frame, true, true)

	// Table selection logic
	table.SetSelectedFunc(func(row, column int) {
		if row == 0 {
			return
		}
		file := &files[row-1]
		details := fmt.Sprintf(
			"File: %s\nSize: %d bytes\nLast Modified: %s",
			file.Name, file.Size, file.Date.Format(time.RFC3339),
		)
		if len(file.Data) > 0 {
			TerminalView(app, pages, file, func() {
				pages.SwitchToPage("main")
				app.SetFocus(table)
			})
			return
		}

		modal := tview.NewModal().
			SetText(details).
			AddButtons([]string{"OK", "CANCEL"}).
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				pages.RemovePage("modal")
				app.SetFocus(table)

				if buttonLabel == "OK" {
					DownloadFile(bucket, file, s3client)
					if len(file.Data) > 0 {
						table.GetCell(row, 3).SetText("Downloaded")
					}
					TerminalView(app, pages, file, func() {
						pages.SwitchToPage("main")
						app.SetFocus(table)
					})
				}
			})

		pages.AddPage("modal", modal, true, true)
		app.SetFocus(modal)

		app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if event.Key() == tcell.KeyEscape {
				pages.RemovePage("modal")
				app.SetFocus(table)
				app.SetInputCapture(nil)
				return nil
			}
			return event
		})
	})

	// Esc quits from table
	table.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			app.Stop()
		}
	})

	// Set up root with pages
	if err := app.SetRoot(pages, true).EnableMouse(true).Run(); err != nil {
		log.Fatal(err)
	}
}

type TerminalStatus struct {
	queryMode     string
	intervalStart time.Time
	intervalEnd   time.Time
	interval      time.Duration
	lookbackDelta int
}

func BuildLeftCol(db *tsdb.DB, bucket string, jobId string, date time.Time) *tview.Flex {
	table := tview.NewTable().
		SetBorders(false).
		SetSelectable(false, false)
	minTime := db.Head().MinTime()
	maxTime := db.Head().MaxTime()
	labels := []string{"Bucket:", "Job:", "Created on:", "Min Time:", "Max:", "Num series:"}
	values := []string{bucket,
		jobId,
		fmt.Sprintf("%s [blue](%d)", date.Format(time.RFC3339), date.UnixMilli()),
		fmt.Sprintf(
			"%s [blue](%d)",
			time.Unix(minTime/1000, (minTime%1000)*int64(time.Millisecond)).Format(time.RFC3339),
			minTime,
		),
		fmt.Sprintf(
			"%s [blue](%d)",
			time.Unix(maxTime/1000, (maxTime%1000)*int64(time.Millisecond)).Format(time.RFC3339),
			maxTime,
		),
		strconv.FormatUint(db.Head().NumSeries(), 10)}

	for i := range labels {
		table.SetCell(i, 0,
			tview.NewTableCell(labels[i]).
				SetTextColor(tcell.ColorOrange).
				SetAlign(tview.AlignLeft).
				SetExpansion(0)) // fixed-ish column for labels

		table.SetCell(i, 1,
			tview.NewTableCell(values[i]).
				SetAlign(tview.AlignLeft).
				SetExpansion(1)) // expands to fill
	}

	leftCol := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(table, 0, 1, false)
	//leftCol.SetBorder(true)

	return leftCol
} // Build once, keep a ref to the table so you can update the value cells later.
func BuildMiddleCol(status *TerminalStatus) (*tview.Flex, *tview.Table) {
	table := tview.NewTable().
		SetBorders(false).
		SetSelectable(false, false)

	labels := []string{
		"Current mode:",
		"Lookback delta:",
		"Interval start:",
		"Interval end:",
		"Interval:",
	}
	values := []string{
		status.queryMode,
		fmt.Sprintf("%ds", status.lookbackDelta),
		fmt.Sprintf("%s [blue](%d)", status.intervalStart.Format(time.RFC3339), status.intervalStart.UnixMilli()),
		fmt.Sprintf("%s [blue](%d)", status.intervalEnd.Format(time.RFC3339), status.intervalEnd.UnixMilli()),
		string(status.interval),
	}

	for i := range labels {
		table.SetCell(i, 0,
			tview.NewTableCell(labels[i]).
				SetTextColor(tcell.ColorOrange).
				SetAlign(tview.AlignLeft).
				SetExpansion(0))

		table.SetCell(i, 1,
			tview.NewTableCell(values[i]).
				SetAlign(tview.AlignLeft).
				SetExpansion(1))
	}

	col := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(table, 0, 1, false)

	return col, table
}

// Call this after ProcessCommand mutates status.
// Only updates the value column (col=1).
func UpdateMiddleCol(tbl *tview.Table, status *TerminalStatus) {
	if tbl == nil {
		return
	}
	set := func(row int, text string) {
		// If the cell exists, update it; otherwise create it.
		if c := tbl.GetCell(row, 1); c != nil {
			c.SetText(text)
			return
		}
		tbl.SetCell(row, 1, tview.NewTableCell(text).SetAlign(tview.AlignLeft).SetExpansion(1))
	}

	set(0, status.queryMode)
	set(1, fmt.Sprintf("%ds", status.lookbackDelta))
	set(2, fmt.Sprintf("%s [blue](%d)", status.intervalStart.Format(time.RFC3339), status.intervalStart.UnixMilli()))
	set(3, fmt.Sprintf("%s [blue](%d)", status.intervalEnd.Format(time.RFC3339), status.intervalEnd.UnixMilli()))
	set(4, string(status.interval))
}
func BuildCommandsCol() *tview.Flex {
	table := tview.NewTable().
		SetBorders(false).
		SetSelectable(false, false)

	cmds := []string{
		"$ exit",
		"$ mode instant/range",
		"$ lookback !val",
		"$ interval start !val",
		"$ interval end !val",
		"$ interval !val",
		"$ metrics !val",
	}
	descs := []string{
		"exit query view",
		"change mode",
		"set lookback delta to !val seconds",
		"set interval start to !val (unix ms)",
		"set interval end to !val (unix ms)",
		"set interval to !val (seconds)",
		"list metrics containing !val",
	}

	for i := range cmds {
		table.SetCell(i, 0,
			tview.NewTableCell(cmds[i]).
				SetTextColor(tcell.ColorGreen).
				SetAlign(tview.AlignLeft).
				SetExpansion(0)) // fixed-ish command column

		table.SetCell(i, 1,
			tview.NewTableCell(descs[i]).
				SetAlign(tview.AlignLeft).
				SetExpansion(1)) // description expands
	}

	col := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(table, 0, 1, false)
	//col.SetBorder(true)
	return col
}
func TerminalView(app *tview.Application, pages *tview.Pages, file *FileItem, onExit func()) {
	status := TerminalStatus{}
	status.queryMode = "instant"
	status.intervalEnd = file.Date
	status.intervalStart = file.Date.Add(-1 * time.Hour)
	status.interval = 300 * time.Second

	ts, err := teststorage.NewWithError()
	if err != nil {
		log.Fatal("Failed to create storage")
	}

	appender := ts.Appender(context.Background())
	ParseSequenceString(appender, file.Data)

	engine := promql.NewEngine(promql.EngineOpts{
		MaxSamples:    10000,
		Timeout:       5 * time.Second,
		LookbackDelta: 5 * time.Minute,
	})
	middleFlex, middleTable := BuildMiddleCol(&status)
	outputView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true)
	outputView.
		SetChangedFunc(func() { outputView.ScrollToEnd(); app.Draw() })

	inputField := tview.NewInputField()
	inputField.
		SetLabel("> ").
		SetFieldWidth(0).
		SetDoneFunc(func(key tcell.Key) {
			if key == tcell.KeyEnter {
				cmd := inputField.GetText()
				if cmd == "" {
					return
				}
				outputView.Write([]byte(fmt.Sprintf("[orange]$ %s", cmd)))
				inputField.SetText("")

				parsedCommand := ProcessCommand(cmd, ts.DB, &status)
				if parsedCommand != "" {
					UpdateMiddleCol(middleTable, &status)
					//appp.Draw()
					outputView.Write([]byte(fmt.Sprintf("\n[green] %s\n", parsedCommand)))
					return
				}
				var (
					query promql.Query
					err   error
				)
				if status.queryMode == "instant" {
					query, err = engine.NewInstantQuery(
						context.Background(),
						ts,
						nil,
						cmd,
						time.Now(),
					)
				} else {
					query, err = engine.NewRangeQuery(
						context.Background(),
						ts,
						nil,
						cmd,
						status.intervalStart,
						status.intervalEnd,
						status.interval,
					)
				}
				if err != nil {
					outputView.Write([]byte(fmt.Sprintf("\n[green]%v\n", err)))
				}
				response := "--no response--"

				if query != nil {
					res := query.Exec(context.Background())
					if res.Err != nil {
						outputView.Write([]byte(fmt.Sprintf("\n[green]%v\n", res.Err)))
						return
					}

					if len(res.Value.String()) > 0 {
						response = res.Value.String()
					}
					outputView.Write([]byte(fmt.Sprintf("\n[green]%s\n", response)))
				}

			}
		})

	// Your existing bordered "terminal" area.
	terminal := tview.NewFlex()
	terminal.
		SetDirection(tview.FlexRow).
		SetBorder(true)
	terminal.
		AddItem(outputView, 0, 1, false).
		AddItem(inputField, 1, 0, true)

		// --- LEFT column (your row1)

	rightCol := tview.NewFlex().
		SetDirection(tview.FlexColumn)
	//rightCol.SetBorder(true) //.

	logo := tview.NewTextView().SetText(header).SetTextColor(tcell.ColorOrange).SetTextAlign(tview.AlignCenter)
	rightCol.AddItem(logo, 0, 1, false)

	// --- Header as 3 columns

	header := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(BuildLeftCol(ts.DB, file.Context, file.Name, file.Date), 0, 25, false).
		AddItem(middleFlex, 0, 25, false).
		AddItem(BuildCommandsCol(), 0, 27, false).
		AddItem(rightCol, 0, 20, true)

	root := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(header, 7, 0, false). // 1 row tall header
		AddItem(terminal, 0, 1, true) // fill the rest with the terminal

	pages.AddAndSwitchToPage("terminal", root, true)
	app.SetFocus(inputField)

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			app.SetInputCapture(nil)
			ts.Close()
			onExit()
			return nil
		}
		return event
	})
}

func ProcessCommand(stdin string, db *tsdb.DB, status *TerminalStatus) string {
	parts := strings.Fields(stdin)
	if len(parts) == 0 {
		return ""
	}

	switch parts[0] {
	case "exit":
		if len(parts) == 1 {
			return "exit"
		}
		return "invalid number of arguments"

	case "mode":
		if len(parts) == 2 {
			// expected: "instant" or "range" (accept whatever you want)
			if parts[1] != "instant" && parts[1] != "range" {
				return "invalid argument"
			}
			status.queryMode = parts[1]
			return "mode " + parts[1]
		}
		return "invalid number of arguments"

	case "lookback":
		if len(parts) == 2 {
			if newLookback, err := strconv.Atoi(parts[1]); err == nil {
				status.lookbackDelta = newLookback
				return "lookback " + parts[1]
			}
			return "failed to parse number"
		}
		return "invalid number of arguments"

	case "interval":
		if len(parts) == 3 && (parts[1] == "start" || parts[1] == "end") {
			val, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil {
				return "invalid number of arguments"
			}
			if parts[1] == "start" {
				status.intervalStart = time.UnixMilli(val)
				return "interval start " + parts[2]
			}
			status.intervalEnd = time.UnixMilli(val)
			return "interval end " + parts[2]
		} else if len(parts) == 2 {

			val, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				return "invalid number of arguments"
			}
			status.interval = time.Second * time.Duration(val)
			return "interval " + parts[1]
		}

		return "invalid arguments"

	case "metrics":
		// "metrics" (no arg) => list all metric names
		// "metrics <substr>"  => list metric names containing <substr>
		if len(parts) == 1 || len(parts) == 2 {

			return ""
		}
		return "invalid number of arguments"
	}

	return ""
}
