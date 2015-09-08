// Copyright 2015, David Howden
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"golang.org/x/net/websocket"

	"tchaik.com/index"
	"tchaik.com/index/cursor"
	"tchaik.com/index/playlist"
	"tchaik.com/player"
)

// Command is a type which is a container for data received from the websocket.
type Command struct {
	Action string
	Data   map[string]interface{}
}

func (c Command) get(f string) (interface{}, error) {
	raw, ok := c.Data[f]
	if !ok {
		return nil, fmt.Errorf("expected '%s' in data map", f)
	}
	return raw, nil
}

func (c Command) getString(f string) (string, error) {
	raw, err := c.get(f)
	if err != nil {
		return "", err
	}

	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("expected '%s' to be of type 'string', got '%T'", f, raw)
	}
	return value, nil
}

func (c Command) getFloat(f string) (float64, error) {
	raw, err := c.get(f)
	if err != nil {
		return 0.0, err
	}

	value, ok := raw.(float64)
	if !ok {
		return 0.0, fmt.Errorf("expected '%s' to be of type 'float64', got '%T'", f, raw)
	}
	return value, nil
}

func (c Command) getInt(f string) (int, error) {
	raw, err := c.getFloat(f)
	if err != nil {
		return 0, err
	}
	return int(raw), nil
}

func (c Command) getBool(f string) (bool, error) {
	raw, err := c.get(f)
	if err != nil {
		return false, err
	}

	value, ok := raw.(bool)
	if !ok {
		return false, fmt.Errorf("expected '%s' to be of type 'bool', got '%T'", f, raw)
	}
	return value, nil
}

func (c Command) getPath(f string) (index.Path, error) {
	raw, err := c.get(f)
	if err != nil {
		return nil, err
	}

	return index.PathFromJSONInterface(raw)
}

type sameSearcher struct {
	index.Searcher
	paths []index.Path
	same  bool
}

func (r *sameSearcher) Search(input string) []index.Path {
	paths := r.Searcher.Search(input)
	r.same = false
	if len(r.paths) == len(paths) {
		r.same = true
		for i, path := range r.paths {
			if path[1] != paths[i][1] {
				r.same = false
				break
			}
		}
	}
	r.paths = paths
	return paths
}

const (
	// Player Actions
	ActionKey    string = "KEY"
	ActionPlayer        = "PLAYER"

	// Path Actions
	ActionRecordPlay   = "RECORD_PLAY"
	ActionSetFavourite = "SET_FAVOURITE"
	ActionSetChecklist = "SET_CHECKLIST"

	// Playlist Actions
	ActionPlaylist = "PLAYLIST"

	// Cursor Actions
	ActionCursor = "CURSOR"

	// Library Actions
	ActionCtrl          = "CTRL"
	ActionFetch         = "FETCH"
	ActionSearch        = "SEARCH"
	ActionFilterList    = "FILTER_LIST"
	ActionFilterPaths   = "FILTER_PATHS"
	ActionFetchPathList = "FETCH_PATHLIST"
)

// NewWebsocketHandler creates a websocket handler for the library, players and history.
func NewWebsocketHandler(l Library, m *Meta, p *player.Players) http.Handler {
	return websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()
		h := &websocketHandler{
			Conn:    ws,
			lib:     l,
			meta:    m,
			players: p,
			searcher: &sameSearcher{
				Searcher: l.searcher,
			},
		}
		h.Handle()
	})
}

type websocketHandler struct {
	*websocket.Conn
	players  *player.Players
	lib      Library
	searcher *sameSearcher
	meta     *Meta

	playerKey string
}

func (h *websocketHandler) Handle() {
	defer h.players.Remove(h.playerKey)

	var err error
	for {
		var c Command
		err = websocket.JSON.Receive(h.Conn, &c)
		if err != nil {
			if err != io.EOF {
				err = fmt.Errorf("receive: %v", err)
			}
			break
		}

		var resp *Response
		switch c.Action {
		// Player actions
		case ActionKey:
			err = h.key(c)

		case ActionPlayer:
			resp, err = h.player(c)

		// Path Actions
		case ActionRecordPlay:
			err = h.recordPlay(c)

		case ActionSetFavourite:
			err = h.setFavourite(c)

		case ActionSetChecklist:
			err = h.setChecklist(c)

		// Playlist Actions
		case ActionPlaylist:
			resp, err = h.playlist(c)

		// Cursor Actions
		case ActionCursor:
			resp, err = h.cursor(c)

		// Library actions
		case ActionFetch:
			resp, err = h.collectionList(c)

		case ActionSearch:
			resp, err = h.search(c)

		case ActionFilterList:
			resp, err = h.filterList(c)

		case ActionFilterPaths:
			resp, err = h.filterPaths(c)

		case ActionFetchPathList:
			resp, err = h.fetchPathList(c)

		default:
			err = fmt.Errorf("unknown action: %v", c.Action)
		}

		if err != nil {
			break
		}

		if resp == nil {
			continue
		}

		err = websocket.JSON.Send(h.Conn, resp)
		if err != nil {
			if err != io.EOF {
				err = fmt.Errorf("send: %v", err)
			}
			break
		}
	}

	if err != nil && err != io.EOF {
		log.Printf("socket error: %v", err)
	}
}

type Response struct {
	Action string
	Data   interface{}
}

func (h *websocketHandler) player(c Command) (*Response, error) {
	action, err := c.getString("action")
	if err != nil {
		return nil, err
	}

	if action == "LIST" {
		return &Response{
			Action: c.Action,
			Data:   h.players.List(),
		}, nil
	}

	key, err := c.getString("key")
	if err != nil {
		return nil, err
	}

	p := h.players.Get(key)
	if p == nil {
		return nil, fmt.Errorf("invalid player key: %v", key)
	}

	r := player.RepAction{
		Action: action,
		Value:  c.Data["value"],
	}
	return nil, r.Apply(p)
}

func (h *websocketHandler) key(c Command) error {
	key, err := c.getString("key")
	if err != nil {
		return err
	}

	h.players.Remove(h.playerKey)
	if key != "" {
		h.players.Add(player.Validated(WebsocketPlayer(key, h.Conn)))
	}
	h.playerKey = key
	return nil
}

func (h *websocketHandler) recordPlay(c Command) error {
	p, err := c.getPath("path")
	if err != nil {
		return err
	}
	return h.meta.history.Add(p)
}

func (h *websocketHandler) setFavourite(c Command) error {
	p, err := c.getPath("path")
	if err != nil {
		return err
	}
	value, err := c.getBool("value")
	if err != nil {
		return err
	}
	return h.meta.favourites.Set(p, value)
}

func (h *websocketHandler) setChecklist(c Command) error {
	p, err := c.getPath("path")
	if err != nil {
		return err
	}
	value, err := c.getBool("value")
	if err != nil {
		return err
	}
	return h.meta.checklist.Set(p, value)
}

func (h *websocketHandler) cursor(c Command) (*Response, error) {
	name, err := c.getString("name")
	if err != nil {
		return nil, err
	}

	action, err := c.getString("action")
	if err != nil {
		return nil, err
	}

	if action != "FETCH" {
		path, _ := c.getPath("path")
		index, _ := c.getInt("index")

		ra := cursor.RepAction{
			Name:   name,
			Action: cursor.Action(action),
			Path:   path,
			Index:  index,
		}

		root := &rootCollection{h.lib.collections["Root"]}
		err = ra.Apply(h.meta.cursors, h.meta.playlists, root)
		if err != nil {
			return nil, err
		}
	}

	return &Response{
		Action: c.Action,
		Data:   h.meta.cursors.Get(name),
	}, nil
}

func (h *websocketHandler) playlist(c Command) (*Response, error) {
	name, err := c.getString("name")
	if err != nil {
		return nil, err
	}

	action, err := c.getString("action")
	if err != nil {
		return nil, err
	}

	if action != "FETCH" {
		path, err := c.getPath("path")
		if err != nil {
			return nil, err
		}
		index, _ := c.getInt("index")

		ra := playlist.RepAction{
			Name:   name,
			Action: playlist.Action(action),
			Path:   path,
			Index:  index,
		}

		err = ra.Apply(h.meta.playlists)
		if err != nil {
			return nil, err
		}
	}

	return &Response{
		Action: c.Action,
		Data:   h.meta.playlists.Get(name),
	}, nil
}

func (h *websocketHandler) collectionList(c Command) (*Response, error) {
	p, err := c.getPath("path")
	if err != nil {
		return nil, err
	}

	if len(p) < 1 {
		return nil, fmt.Errorf("invalid path: %v\n", p)
	}

	root := h.lib.collections[string(p[0])]
	if root == nil {
		return nil, fmt.Errorf("unknown collection: %#v", p[0])
	}
	g, err := h.lib.Fetch(root, p[1:])
	if err != nil {
		return nil, fmt.Errorf("error in Fetch: %v (path: %#v)", err, p[1:])
	}

	g = h.meta.annotateFavourites(p, g)
	g = h.meta.annotateChecklist(p, g)

	return &Response{
		Action: c.Action,
		Data: struct {
			Path index.Path
			Item group
		}{
			p,
			g,
		},
	}, nil
}

func (h *websocketHandler) filterList(c Command) (*Response, error) {
	filterName, err := c.getString("name")
	if err != nil {
		return nil, err
	}

	filterItems, ok := h.lib.filters[filterName]
	if !ok {
		return nil, fmt.Errorf("invalid filter name: %#v", filterName)
	}

	filterNames := make([]string, len(filterItems))
	for i, x := range filterItems {
		filterNames[i] = x.Name()
	}
	return &Response{
		Action: c.Action,
		Data: struct {
			Name  string
			Items []string
		}{
			Name:  filterName,
			Items: filterNames,
		},
	}, nil
}

func (h *websocketHandler) filterPaths(c Command) (*Response, error) {
	path, err := c.getPath("path")
	if err != nil {
		return nil, err
	}

	filterName, err := c.getString("name")
	if err != nil {
		return nil, err
	}

	filterItems, ok := h.lib.filters[filterName]
	if !ok {
		return nil, fmt.Errorf("invalid filter name: %#v", filterName)
	}

	if len(path) != 1 {
		return nil, fmt.Errorf("invalid path: %#v", path)
	}
	name := string(path[0])

	var item index.FilterItem
	for _, x := range filterItems {
		if x.Name() == name {
			item = x
			break
		}
	}
	if item == nil {
		return nil, fmt.Errorf("invalid filter item: %#v", name)
	}

	return &Response{
		Action: c.Action,
		Data: struct {
			Path  index.Path
			Paths group
		}{
			Path:  index.PathFromStringSlice([]string{filterName, name}),
			Paths: h.lib.ExpandPaths(item.Paths()),
		},
	}, nil
}

// Lister is an interface which defines the List method.
type Lister interface {
	// List returns a list of index.Paths.
	List() []index.Path
}

// NB: matches paths based only on the first two keys.
func filterByRootLister(l Lister, paths []index.Path) []index.Path {
	exp := make(map[string]bool)
	for _, p := range l.List() {
		if len(p) > 1 {
			exp[fmt.Sprintf("%v", p[:2])] = true
		}
	}

	result := make([]index.Path, 0, len(paths))
	for _, p := range paths {
		if exp[fmt.Sprintf("%v", p)] {
			result = append(result, p)
		}
	}
	return result
}

func (h *websocketHandler) fetchPathList(c Command) (*Response, error) {
	name, err := c.getString("name")
	if err != nil {
		return nil, err
	}

	var paths []index.Path
	switch name {
	case "recent":
		paths = h.lib.recent

	case "favourite":
		paths = index.CollectionPaths(h.lib.collections["Root"], []index.Key{"Root"})
		paths = filterByRootLister(h.meta.favourites, paths)

	case "checklist":
		paths = index.CollectionPaths(h.lib.collections["Root"], []index.Key{"Root"})
		paths = filterByRootLister(h.meta.checklist, paths)
	}

	return &Response{
		Action: c.Action,
		Data: struct {
			Name string
			Data group
		}{
			Name: name,
			Data: h.lib.ExpandPaths(paths),
		},
	}, nil
}

func (h *websocketHandler) search(c Command) (*Response, error) {
	input, err := c.getString("input")
	if err != nil {
		return nil, err
	}

	paths := h.searcher.Search(input)
	if h.searcher.same {
		return nil, nil
	}

	return &Response{
		Action: c.Action,
		Data:   h.lib.ExpandPaths(paths),
	}, nil
}

// WebsocketPlayer creates a player.Player which sends commands down the websocket.Conn when
// player.Player methods are called.
func WebsocketPlayer(key string, ws *websocket.Conn) player.Player {
	repFn := func(data interface{}) {
		websocket.JSON.Send(ws, &Response{
			Action: ActionCtrl,
			Data:   data,
		})
	}
	return player.NewRep(key, repFn)
}
