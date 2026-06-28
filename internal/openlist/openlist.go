package openlist

import "context"

// --- Types ---

// FileInfo represents a file or directory entry returned by the OpenList API.
type FileInfo struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	IsDir    bool   `json:"is_dir"`
	Modified string `json:"modified"`
}

// ListResult contains the response from a list operation.
type ListResult struct {
	Content []FileInfo `json:"content"`
	Total   int        `json:"total"`
}

// listRequest is the JSON body sent to /api/fs/list.
type listRequest struct {
	Path     string `json:"path"`
	Password string `json:"password"`
	Page     int    `json:"page"`
	PerPage  int    `json:"per_page"`
	Refresh  bool   `json:"refresh"`
}

// getRequest is the JSON body sent to /api/fs/get.
type getRequest struct {
	Path     string `json:"path"`
	Password string `json:"password"`
}

// removeRequest is the JSON body sent to /api/fs/remove.
type removeRequest struct {
	Path     string `json:"path"`
	Password string `json:"password"`
}

// --- API Methods ---

// listPerPage is the page size used when paginating directory listings.
const listPerPage = 1000

// List returns the contents of a directory. For directories with many entries,
// it paginates through all pages automatically.
func (c *Client) List(ctx context.Context, path string) (*ListResult, error) {
	req := listRequest{
		Path:     path,
		Password: c.password,
		Page:     1,
		PerPage:  listPerPage,
		Refresh:  false,
	}

	var result ListResult
	if err := c.request(ctx, "/api/fs/list", &req, &result); err != nil {
		return nil, err
	}

	// Handle pagination if total > returned content. Stop early if a page
	// returns no new entries, so a stale or inconsistent Total can never cause
	// an infinite request loop. (Any non-empty page grows allContent, so the
	// length condition is guaranteed to terminate; only a zero-progress page
	// could loop forever, which the break below prevents.)
	allContent := make([]FileInfo, 0, result.Total)
	allContent = append(allContent, result.Content...)

	for len(allContent) < result.Total {
		req.Page++
		var pageResult ListResult
		if err := c.request(ctx, "/api/fs/list", &req, &pageResult); err != nil {
			return nil, err
		}
		if len(pageResult.Content) == 0 {
			// Server reported more entries than it actually returns; stop rather
			// than loop forever.
			break
		}
		allContent = append(allContent, pageResult.Content...)
	}

	result.Content = allContent
	return &result, nil
}

// Get returns detailed information about a single file or directory.
func (c *Client) Get(ctx context.Context, path string) (*FileInfo, error) {
	req := getRequest{
		Path:     path,
		Password: c.password,
	}

	var info FileInfo
	if err := c.request(ctx, "/api/fs/get", &req, &info); err != nil {
		return nil, err
	}

	return &info, nil
}

// Delete removes a file or directory. If the path is a directory, it must be
// empty for the deletion to succeed.
func (c *Client) Delete(ctx context.Context, path string) error {
	req := removeRequest{
		Path:     path,
		Password: c.password,
	}

	return c.request(ctx, "/api/fs/remove", &req, nil)
}
