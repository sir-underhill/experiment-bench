// Experiment Bench — a LAN-hosted Q/A that turns the "experiments, not habits"
// framework (from Mia's "I Spent 3 Months Being Curious Instead Of Scrolling"
// experiment compilation) into experiment cards you can actually run.
//
// Stack: Go stdlib net/http + html/template, htmx v4, missing.css.
package main

import (
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed templates static
var content embed.FS

// ---------------------------------------------------------------- state ----

type Experiment struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Area    string `json:"area"`
	ResearchQ string `json:"research_q"`
	Action  string `json:"action"`
	Duration string `json:"duration"`
	DataPlan string `json:"data_plan"`
	Started string `json:"started"`
	Review  string `json:"review"`
	Status  string `json:"status"` // running | analyzing | kept | revised | dropped
	Notes   string `json:"notes"`
}

type State struct {
	Answers     map[string]string `json:"answers"`
	Experiments []Experiment      `json:"experiments"`
	Ideas       string            `json:"ideas"`
}

var (
	mu        sync.Mutex
	state     = State{Answers: map[string]string{}}
	statePath string
)

func loadState() {
	if d := os.Getenv("DATA_DIR"); d != "" {
		statePath = filepath.Join(d, "state.json")
	} else {
		statePath = "data/state.json"
	}
	b, err := os.ReadFile(statePath)
	if err == nil {
		json.Unmarshal(b, &state)
	}
	if state.Answers == nil {
		state.Answers = map[string]string{}
	}
}

func saveState() {
	os.MkdirAll(filepath.Dir(statePath), 0o755)
	b, _ := json.MarshalIndent(state, "", "  ")
	os.WriteFile(statePath, b, 0o644)
}

// -------------------------------------------------------------- template ----

var funcs = template.FuncMap{
	"safeHTML": func(s string) template.HTML { return template.HTML(s) },
	"list": func(vs ...string) []string { return vs },
}

var tpl *template.Template

func parseTemplates() {
	tpl = template.Must(template.New("").Funcs(funcs).ParseFS(content, "templates/*.html"))
}

// -------------------------------------------------------------- handlers ----

func page(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template %s: %v", name, err)
		http.Error(w, "template error", 500)
	}
}

func idx(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	mu.Lock(); n := len(state.Experiments); mu.Unlock()
	page(w, "home.html", map[string]any{"ExpCount": n})
}

func qa(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	_, done := state.Answers["__finished"]
	answers := state.Answers
	mu.Unlock()
	stage := 1
	if done {
		stage = 5
	}
	page(w, "qa.html", map[string]any{"Stage": stage, "Answers": answers})
}

// answers posted per stage by htmx; responds with the next stage's partial.
func postAnswers(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	mu.Lock()
	stage := r.Form.Get("stage")
	for k, v := range r.Form {
		if strings.HasPrefix(k, "q_") && len(v) > 0 && v[0] != "" {
			state.Answers[k[2:]] = strings.Join(v, ", ")
		}
	}
	n := stage
	answers := state.Answers
	mu.Unlock()
	next := map[string]int{"1": 2, "2": 3, "3": 4, "4": 5}[n]
	if next == 5 {
		mu.Lock(); state.Answers["__finished"] = "1"; saveState(); answers = state.Answers; mu.Unlock()
	}
	page(w, "stage", map[string]any{"Stage": next, "Answers": answers})
}

// draft builds a prefill for the board's add-form from the Q/A answers.
func draftPrefill() map[string]string {
	mu.Lock(); defer mu.Unlock()
	name := "Q/A draft experiment"
	if a := state.Answers["nagging"]; len(a) > 40 {
		name = a[:40] + "…"
	} else if a != "" {
		name = a
	}
	return map[string]string{
		"name":       name,
		"area":       state.Answers["areas"],
		"research_q": state.Answers["research"],
		"action":     state.Answers["action"],
		"duration":   state.Answers["duration"],
		"data_plan":  state.Answers["data"],
	}
}

func experiments(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		r.ParseForm()
		exp := Experiment{
			ID:        fmtTime("20060102150405"),
			Name:      r.Form.Get("name"),
			Area:      r.Form.Get("area"),
			ResearchQ: r.Form.Get("research_q"),
			Action:    r.Form.Get("action"),
			Duration:  r.Form.Get("duration"),
			DataPlan:  r.Form.Get("data_plan"),
			Status:    "running",
			Started:   fmtTime("2006-01-02"),
			Review:    reviewDate(r.Form.Get("duration")),
		}
		mu.Lock()
		if exp.Name != "" {
			state.Experiments = append(state.Experiments, exp)
			saveState()
		}
		exps := state.Experiments
		mu.Unlock()
		if r.Header.Get("HX-Request") != "" {
			page(w, "board", boardData(nil, exps))
		} else {
			http.Redirect(w, r, "/experiments", http.StatusSeeOther)
		}
		return
	}
	prefill := map[string]string{}
	if r.URL.Query().Get("draft") == "1" {
		prefill = draftPrefill()
	}
	mu.Lock(); exps := state.Experiments; mu.Unlock()
	page(w, "experiments.html", boardData(prefill, exps))
}

func boardData(prefill map[string]string, exps []Experiment) map[string]any {
	if prefill == nil {
		prefill = map[string]string{}
	}
	return map[string]any{"Experiments": exps, "Prefill": prefill}
}

func expAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/experiments/"), "/")
	id := parts[0]
	mu.Lock()
	for i := range state.Experiments {
		if state.Experiments[i].ID == id {
			e := &state.Experiments[i]
			if len(parts) > 1 {
				switch parts[1] {
				case "delete":
					state.Experiments = append(state.Experiments[:i], state.Experiments[i+1:]...)
				case "status":
					e.Status = map[string]string{
						"running": "analyzing", "analyzing": "kept", "kept": "revised",
						"revised": "dropped", "dropped": "running"}[e.Status]
				case "notes":
					r.ParseForm()
					e.Notes = r.Form.Get("notes")
				}
			}
		}
	}
	saveState()
	exps := state.Experiments
	mu.Unlock()
	if r.Header.Get("HX-Request") != "" {
		page(w, "board", boardData(nil, exps))
	} else {
		http.Redirect(w, r, "/experiments", http.StatusSeeOther)
	}
}

func printView(w http.ResponseWriter, r *http.Request) {
	mu.Lock(); defer mu.Unlock()
	page(w, "print.html", map[string]any{
		"Experiments": state.Experiments,
		"Today":       time.Now().Format("Mon 2 Jan 2006"),
	})
}

func reset(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	state.Answers = map[string]string{}
	state.Ideas = ""
	saveState()
	mu.Unlock()
	http.Redirect(w, r, "/qa", http.StatusSeeOther)
}

func fmtTime(layout string) string { return time.Now().Format(layout) }

func reviewDate(duration string) string {
	days := map[string]int{"2 weeks": 14, "1 month": 30, "3 months": 90, "quarterly": 90}
	if d, ok := days[duration]; ok {
		return time.Now().AddDate(0, 0, d).Format("2006-01-02")
	}
	return ""
}

// ------------------------------------------------------------------ main ----

func main() {
	loadState()
	parseTemplates()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", idx)
	mux.HandleFunc("GET /qa", qa)
	mux.HandleFunc("POST /answers", postAnswers)
	mux.HandleFunc("/experiments", experiments)
	mux.HandleFunc("/experiments/", expAction)
	mux.HandleFunc("GET /print", printView)
	mux.HandleFunc("POST /reset", reset)
	mux.Handle("GET /static/", http.FileServerFS(content))

	bind := os.Getenv("BIND")
	if bind == "" {
		bind = "0.0.0.0"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8321"
	}
	addr := net.JoinHostPort(bind, port)
	log.Printf("experiment bench listening on http://%s (LAN: http://%s:%s)", addr, lanIP(), port)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func lanIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
