package jenkins

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)


const cacheTTL = 30 * time.Second

type cacheEntry struct {
	jobs []Job
	at   time.Time
}


// ErrUnauthorized is returned when Jenkins responds with 401 or 403.
var ErrUnauthorized = errors.New("unauthorized: token expired or invalid credentials")

type Client struct {
	baseURL    string
	username   string
	apiToken   string
	httpClient *http.Client
	mu         sync.Mutex
	cache      map[string]cacheEntry
}

func NewClient(baseURL, username, apiToken string) *Client {
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache: make(map[string]cacheEntry),
	}
}

func (c *Client) cachedJobs(key string) ([]Job, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[key]
	if !ok || time.Since(e.at) > cacheTTL {
		return nil, false
	}
	return e.jobs, true
}

func (c *Client) setCached(key string, jobs []Job) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = cacheEntry{jobs: jobs, at: time.Now()}
}

func (c *Client) InvalidateCache(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, key)
}

// Job represents a Jenkins job or folder
type Job struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	Color     string `json:"color"`
	Jobs      []Job  `json:"jobs"`
	Class     string `json:"_class"`
	LastBuild *Build `json:"lastBuild"`
}

// JobDetail contains job metadata and build history
type JobDetail struct {
	Name        string  `json:"name"`
	URL         string  `json:"url"`
	Color       string  `json:"color"`
	Buildable   bool    `json:"buildable"`
	LastBuild   *Build  `json:"lastBuild"`
	Builds      []Build `json:"builds"`
	Description string  `json:"description"`
}

// BuildCause holds the cause info for a build (user-triggered or upstream)
type BuildCause struct {
	UpstreamBuild   int    `json:"upstreamBuild"`
	UpstreamProject string `json:"upstreamProject"`
	ShortDesc       string `json:"shortDescription"`
	UserID          string `json:"userId"`
	UserName        string `json:"userName"`
}

// BuildAction wraps the causes array from Jenkins actions
type BuildAction struct {
	Causes []BuildCause `json:"causes"`
}

// Build represents a Jenkins build
type Build struct {
	Number      int           `json:"number"`
	URL         string        `json:"url"`
	Result      string        `json:"result"`
	Building    bool          `json:"building"`
	Timestamp   int64         `json:"timestamp"`
	Duration    int64         `json:"duration"`
	DisplayName string        `json:"displayName"`
	Actions     []BuildAction `json:"actions"`
}

// UpstreamBuildNumber returns the upstream (triggering) build number, or 0
func (b Build) UpstreamBuildNumber() int {
	for _, a := range b.Actions {
		for _, c := range a.Causes {
			if c.UpstreamBuild > 0 {
				return c.UpstreamBuild
			}
		}
	}
	return 0
}

// TriggeredBy returns the human-readable name of who/what triggered this build
func (b Build) TriggeredBy() string {
	for _, a := range b.Actions {
		for _, c := range a.Causes {
			if c.UserName != "" {
				return c.UserName
			}
			if c.UserID != "" {
				return c.UserID
			}
			if c.UpstreamProject != "" {
				// "Production/backend/api-service/Build" → "api-service/Build"
				parts := strings.Split(c.UpstreamProject, "/")
				if len(parts) >= 2 {
					return strings.Join(parts[len(parts)-2:], "/")
				}
				return c.UpstreamProject
			}
		}
	}
	return ""
}

// ParamDef is a Jenkins job parameter definition
type ParamDef struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// CrumbResponse holds the CSRF crumb
type CrumbResponse struct {
	Crumb             string `json:"crumb"`
	CrumbRequestField string `json:"crumbRequestField"`
}

// RootResponse is the top-level Jenkins API response
type RootResponse struct {
	Jobs []Job `json:"jobs"`
}

func (c *Client) newRequest(method, path string, body io.Reader) (*http.Request, error) {
	u := c.baseURL + path
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.apiToken)
	return req, nil
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		resp.Body.Close()
		return nil, fmt.Errorf("%w (HTTP %d)", ErrUnauthorized, resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return resp, nil
}

func (c *Client) getCrumb() (*CrumbResponse, error) {
	req, err := c.newRequest("GET", "/crumbIssuer/api/json", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var crumb CrumbResponse
	if err := json.NewDecoder(resp.Body).Decode(&crumb); err != nil {
		return nil, err
	}
	return &crumb, nil
}

// GetJobs fetches the top-level job/folder list
func (c *Client) GetJobs() ([]Job, error) {
	const key = "__root__"
	if jobs, ok := c.cachedJobs(key); ok {
		return jobs, nil
	}

	req, err := c.newRequest("GET", "/api/json?tree=jobs[name,url,color,_class,jobs[name,url,color,_class]]", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var root RootResponse
	if err := json.NewDecoder(resp.Body).Decode(&root); err != nil {
		return nil, err
	}
	c.setCached(key, root.Jobs)
	return root.Jobs, nil
}

// GetJobsInFolder fetches jobs inside a folder (environment)
func (c *Client) GetJobsInFolder(folderPath string) ([]Job, error) {
	if jobs, ok := c.cachedJobs(folderPath); ok {
		return jobs, nil
	}

	apiPath := "/job/" + strings.Join(strings.Split(folderPath, "/"), "/job/") +
		"/api/json?tree=jobs[name,url,color,_class,lastBuild[number,result,building,duration,actions[causes[userId,userName,upstreamBuild,upstreamProject]]]]"

	req, err := c.newRequest("GET", apiPath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var folder struct {
		Jobs []Job `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&folder); err != nil {
		return nil, err
	}
	c.setCached(folderPath, folder.Jobs)
	return folder.Jobs, nil
}

// GetJobDetail fetches full job details including build history
func (c *Client) GetJobDetail(jobPath string) (*JobDetail, error) {
	apiPath := buildJobAPIPath(jobPath) +
		"/api/json?tree=name,url,color,buildable,description,lastBuild[number,url,result,building,timestamp,duration,displayName,actions[causes[upstreamBuild,upstreamProject,shortDescription]]],builds[number,url,result,building,timestamp,duration,displayName,actions[causes[upstreamBuild,upstreamProject,shortDescription]]]{0,15}"

	req, err := c.newRequest("GET", apiPath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var detail JobDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

// GetLastBuild fetches last build info (for running check)
func (c *Client) GetLastBuild(jobPath string) (*Build, error) {
	apiPath := buildJobAPIPath(jobPath) + "/lastBuild/api/json?tree=number,result,building,timestamp,duration,displayName"

	req, err := c.newRequest("GET", apiPath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var build Build
	if err := json.NewDecoder(resp.Body).Decode(&build); err != nil {
		return nil, err
	}
	return &build, nil
}

// GetJobParamDefinitions returns the parameter definitions for a job
func (c *Client) GetJobParamDefinitions(jobPath string) ([]ParamDef, error) {
	apiPath := buildJobAPIPath(jobPath) + "/api/json?tree=property[parameterDefinitions[name,type]]"
	req, err := c.newRequest("GET", apiPath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Property []struct {
			ParamDefs []ParamDef `json:"parameterDefinitions"`
		} `json:"property"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	var all []ParamDef
	for _, p := range result.Property {
		all = append(all, p.ParamDefs...)
	}
	return all, nil
}

// TriggerBuild triggers a build for the given job path
func (c *Client) TriggerBuild(jobPath string) error {
	crumb, err := c.getCrumb()
	if err != nil {
		return fmt.Errorf("getting crumb: %w", err)
	}

	apiPath := buildJobAPIPath(jobPath) + "/build"
	req, err := c.newRequest("POST", apiPath, strings.NewReader(""))
	if err != nil {
		return err
	}
	req.Header.Set(crumb.CrumbRequestField, crumb.Crumb)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// TriggerBuildWithParams triggers a build with parameters
func (c *Client) TriggerBuildWithParams(jobPath string, params map[string]string) error {
	crumb, err := c.getCrumb()
	if err != nil {
		return fmt.Errorf("getting crumb: %w", err)
	}

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}

	apiPath := buildJobAPIPath(jobPath) + "/buildWithParameters"
	req, err := c.newRequest("POST", apiPath, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set(crumb.CrumbRequestField, crumb.Crumb)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// GetBuildLog fetches the console output for a specific build
func (c *Client) GetBuildLog(jobPath string, buildNumber int) (string, error) {
	apiPath := fmt.Sprintf("%s/%d/consoleText", buildJobAPIPath(jobPath), buildNumber)

	req, err := c.newRequest("GET", apiPath, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// GetBuildLogStream returns a reader for streaming build logs
func (c *Client) GetBuildLogStream(jobPath string, buildNumber int, start int64) (string, int64, bool, error) {
	apiPath := fmt.Sprintf("%s/%d/logText/progressiveText?start=%d", buildJobAPIPath(jobPath), buildNumber, start)

	req, err := c.newRequest("GET", apiPath, nil)
	if err != nil {
		return "", 0, false, err
	}
	resp, err := c.do(req)
	if err != nil {
		return "", 0, false, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, false, err
	}

	// X-More-Data header indicates if build is still running
	moreData := resp.Header.Get("X-More-Data") == "true"
	// X-Text-Size is the next start position
	var nextStart int64
	fmt.Sscanf(resp.Header.Get("X-Text-Size"), "%d", &nextStart)

	return string(body), nextStart, moreData, nil
}

// Validate tests the connection and credentials
func (c *Client) Validate() error {
	req, err := c.newRequest("GET", "/api/json?tree=nodeName", nil)
	if err != nil {
		return err
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// ColorToStatus converts Jenkins color to a human-readable status
func ColorToStatus(color string) string {
	switch {
	case color == "blue":
		return "SUCCESS"
	case color == "blue_anime":
		return "RUNNING"
	case color == "red":
		return "FAILED"
	case color == "red_anime":
		return "RUNNING"
	case color == "yellow":
		return "UNSTABLE"
	case color == "yellow_anime":
		return "RUNNING"
	case color == "grey", color == "disabled":
		return "DISABLED"
	case color == "aborted":
		return "ABORTED"
	case color == "notbuilt":
		return "NOT BUILT"
	default:
		return "UNKNOWN"
	}
}

// ColorToIcon converts Jenkins color to an icon
func ColorToIcon(color string) string {
	switch {
	case color == "blue":
		return "✓"
	case color == "blue_anime":
		return "⟳"
	case color == "red":
		return "✗"
	case color == "red_anime":
		return "⟳"
	case color == "yellow":
		return "⚠"
	case color == "yellow_anime":
		return "⟳"
	case color == "grey", color == "disabled":
		return "○"
	case color == "aborted":
		return "⊘"
	case strings.HasSuffix(color, "_anime"):
		return "⟳"
	default:
		return "?"
	}
}

// IsRunning returns true if the color indicates a running build
func IsRunning(color string) bool {
	return strings.HasSuffix(color, "_anime")
}

// FormatDuration formats milliseconds into human-readable duration
func FormatDuration(ms int64) string {
	if ms <= 0 {
		return "-"
	}
	d := time.Duration(ms) * time.Millisecond
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// FormatTimestamp formats a Unix ms timestamp
func FormatTimestamp(ts int64) string {
	if ts <= 0 {
		return "-"
	}
	t := time.Unix(ts/1000, 0)
	return t.Format("2006-01-02 15:04")
}

// BuildElapsed returns how long a running build has been going
func BuildElapsed(ts int64) string {
	if ts <= 0 {
		return "-"
	}
	start := time.Unix(ts/1000, 0)
	elapsed := time.Since(start)
	return FormatDuration(int64(elapsed / time.Millisecond))
}

// RunningBuild holds info about a currently running build across the tree
type RunningBuild struct {
	EnvName string
	JobPath string
	JobName string
	Build   Build
}

// GetRunningBuilds reads the Jenkins computer/executor API — the same data
// shown in the "Build Executor Status" sidebar. Single API call, instant.
func (c *Client) GetRunningBuilds() ([]RunningBuild, error) {
	apiPath := "/computer/api/json?tree=computer[executors[currentExecutable[url,number,building,timestamp,duration,displayName]],oneOffExecutors[currentExecutable[url,number,building,timestamp,duration,displayName]]]"

	req, err := c.newRequest("GET", apiPath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	type executable struct {
		URL         string `json:"url"`
		Number      int    `json:"number"`
		Building    bool   `json:"building"`
		Timestamp   int64  `json:"timestamp"`
		Duration    int64  `json:"duration"`
		DisplayName string `json:"displayName"`
	}
	type executor struct {
		CurrentExecutable *executable `json:"currentExecutable"`
	}
	type computer struct {
		Executors    []executor `json:"executors"`
		OneOffExec   []executor `json:"oneOffExecutors"`
	}
	var result struct {
		Computer []computer `json:"computer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var running []RunningBuild
	for _, comp := range result.Computer {
		allExec := append(comp.Executors, comp.OneOffExec...)
		for _, ex := range allExec {
			e := ex.CurrentExecutable
			if e == nil || !e.Building {
				continue
			}
			// URL: https://jenkins/job/Production/job/backend/job/api-service/job/Build/42/
			jobPath, jobName, envName := parseJobURL(c.baseURL, e.URL)
			running = append(running, RunningBuild{
				EnvName: envName,
				JobPath: jobPath,
				JobName: jobName,
				Build: Build{
					Number:      e.Number,
					Building:    e.Building,
					Timestamp:   e.Timestamp,
					Duration:    e.Duration,
					DisplayName: e.DisplayName,
				},
			})
		}
	}
	return running, nil
}

// parseJobURL converts a Jenkins build URL to a job path.
// e.g. ".../job/Production/job/backend/job/api-service/job/Build/42/"
//
//	→ jobPath="Production/backend/api-service/Build", jobName="api-service/Build", envName="Production"
func parseJobURL(baseURL, buildURL string) (jobPath, jobName, envName string) {
	// Strip base URL prefix
	path := strings.TrimPrefix(buildURL, baseURL)
	// Split on "/job/" to get path segments
	parts := strings.Split(path, "/job/")
	var segments []string
	for _, p := range parts {
		p = strings.Trim(p, "/")
		if p == "" {
			continue
		}
		// Last segment may contain the build number — drop it
		if _, err := fmt.Sscanf(p, "%d", new(int)); err == nil {
			continue
		}
		segments = append(segments, p)
	}
	if len(segments) == 0 {
		return "", "", ""
	}
	jobPath = strings.Join(segments, "/")
	envName = segments[0]
	if len(segments) >= 2 {
		jobName = strings.Join(segments[len(segments)-2:], "/")
	} else {
		jobName = segments[len(segments)-1]
	}
	return
}

// IsFolder returns true if the job is a folder/multibranch
func IsFolder(j Job) bool {
	if strings.Contains(j.Class, "Folder") ||
		strings.Contains(j.Class, "MultiBranchProject") ||
		strings.Contains(j.Class, "OrganizationFolder") ||
		strings.Contains(j.Class, "Organization") ||
		len(j.Jobs) > 0 {
		return true
	}
	// Jobs always have a color; folders don't
	return j.Color == "" && j.Class == ""
}

func buildJobAPIPath(jobPath string) string {
	parts := strings.Split(jobPath, "/")
	return "/job/" + strings.Join(parts, "/job/")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
