package subsonic

// This file holds the response types and the browse and playback methods.
// The JSON responses wrap all data inside a "subsonic-response" object.

import (
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// Error is a Subsonic API error.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("subsonic error %d: %s", e.Code, e.Message)
}

// baseResponse holds the fields that every response shares.
type baseResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Error   *Error `json:"error"`
}

// Artist is one artist in the library.
type Artist struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	AlbumCount int    `json:"albumCount"`
}

// Album is one album in the library.
type Album struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Artist   string `json:"artist"`
	ArtistID string `json:"artistId"`
	Year     int    `json:"year"`
	Genre    string `json:"genre"`
	Songs    []Song `json:"song"`
}

// Song is one track in the library.
type Song struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Album      string `json:"album"`
	Artist     string `json:"artist"`
	Track      int    `json:"track"`
	DiscNumber int    `json:"discNumber"`
	Year       int    `json:"year"`
	Duration   int    `json:"duration"` // seconds
	Suffix     string `json:"suffix"`   // file extension, e.g. "mp3"
	CoverArt   string `json:"coverArt"`
	Genre      string `json:"genre"`
	Path       string `json:"path"`
	BitRate    int    `json:"bitRate"`
	Size       int64  `json:"size"`
	// Comment is the free-text comment tag. Navidrome returns it as an
	// OpenSubsonic field in search3 and getSong results.
	Comment string `json:"comment"`
	// Starred is the time the user starred this song, or empty when the
	// song is not starred. The value is an ISO timestamp from the server.
	Starred string `json:"starred"`
	// UserRating is the rating the user gave this song, 0 to 5. A value
	// of 0 means unrated. A value of 1 marks a disliked ("thumbs down")
	// song. The server persists it. tuiplay reads it from getSong and
	// search3 results.
	UserRating int `json:"userRating"`
}

// IsStarred reports whether the user has starred this song.
func (s Song) IsStarred() bool {
	return s.Starred != ""
}

// Ping tests the connection and the credentials.
func (c *Client) Ping() error {
	var r struct {
		Response baseResponse `json:"subsonic-response"`
	}
	if err := c.get("ping", nil, &r); err != nil {
		return err
	}
	if r.Response.Status != "ok" {
		if r.Response.Error != nil {
			return r.Response.Error
		}
		return fmt.Errorf("ping failed with status %q", r.Response.Status)
	}
	return nil
}

// GetArtists returns all artists, organized by ID3 tags.
func (c *Client) GetArtists() ([]Artist, error) {
	var r struct {
		Response struct {
			baseResponse
			Artists struct {
				Index []struct {
					Name   string   `json:"name"`
					Artist []Artist `json:"artist"`
				} `json:"index"`
			} `json:"artists"`
		} `json:"subsonic-response"`
	}
	if err := c.get("getArtists", nil, &r); err != nil {
		return nil, err
	}
	if r.Response.Error != nil {
		return nil, r.Response.Error
	}
	var out []Artist
	for _, idx := range r.Response.Artists.Index {
		out = append(out, idx.Artist...)
	}
	return out, nil
}

// GetArtist returns one artist and its albums, organized by ID3 tags.
func (c *Client) GetArtist(id string) ([]Album, error) {
	extra := url.Values{}
	extra.Set("id", id)
	var r struct {
		Response struct {
			baseResponse
			Artist struct {
				Album []Album `json:"album"`
			} `json:"artist"`
		} `json:"subsonic-response"`
	}
	if err := c.get("getArtist", extra, &r); err != nil {
		return nil, err
	}
	if r.Response.Error != nil {
		return nil, r.Response.Error
	}
	return r.Response.Artist.Album, nil
}

// GetAlbum returns one album and its songs, organized by ID3 tags.
func (c *Client) GetAlbum(id string) (Album, error) {
	extra := url.Values{}
	extra.Set("id", id)
	var r struct {
		Response struct {
			baseResponse
			Album Album `json:"album"`
		} `json:"subsonic-response"`
	}
	if err := c.get("getAlbum", extra, &r); err != nil {
		return Album{}, err
	}
	if r.Response.Error != nil {
		return Album{}, r.Response.Error
	}
	return r.Response.Album, nil
}

// Scrobble registers the playback of one song.
// If submission is true, this is a final submission. If submission is
// false, this is a now-playing notification.
func (c *Client) Scrobble(id string, submission bool) error {
	extra := url.Values{}
	extra.Set("id", id)
	extra.Set("time", fmt.Sprintf("%d", time.Now().UnixMilli()))
	if submission {
		extra.Set("submission", "true")
	} else {
		extra.Set("submission", "false")
	}
	var r struct {
		Response baseResponse `json:"subsonic-response"`
	}
	if err := c.get("scrobble", extra, &r); err != nil {
		return err
	}
	if r.Response.Error != nil {
		return r.Response.Error
	}
	return nil
}

// Genre is one genre in the library.
type Genre struct {
	Name       string `json:"value"`
	AlbumCount int    `json:"albumCount"`
	SongCount  int    `json:"songCount"`
}

// GetGenres returns all genres.
func (c *Client) GetGenres() ([]Genre, error) {
	var r struct {
		Response struct {
			baseResponse
			Genres struct {
				Genre []Genre `json:"genre"`
			} `json:"genres"`
		} `json:"subsonic-response"`
	}
	if err := c.get("getGenres", nil, &r); err != nil {
		return nil, err
	}
	if r.Response.Error != nil {
		return nil, r.Response.Error
	}
	return r.Response.Genres.Genre, nil
}

// albumListMax is the largest page size that Subsonic allows for a
// getAlbumList2 request.
const albumListMax = 500

// getAlbumList2 calls getAlbumList2 once with the given extra parameters.
func (c *Client) getAlbumList2(extra url.Values) ([]Album, error) {
	var r struct {
		Response struct {
			baseResponse
			AlbumList2 struct {
				Album []Album `json:"album"`
			} `json:"albumList2"`
		} `json:"subsonic-response"`
	}
	if err := c.get("getAlbumList2", extra, &r); err != nil {
		return nil, err
	}
	if r.Response.Error != nil {
		return nil, r.Response.Error
	}
	return r.Response.AlbumList2.Album, nil
}

// GetAllAlbums returns every album in alphabetical order. It pages through
// the whole library.
func (c *Client) GetAllAlbums() ([]Album, error) {
	var out []Album
	for offset := 0; ; offset += albumListMax {
		extra := url.Values{}
		extra.Set("type", "alphabeticalByName")
		extra.Set("size", fmt.Sprintf("%d", albumListMax))
		extra.Set("offset", fmt.Sprintf("%d", offset))
		page, err := c.getAlbumList2(extra)
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) < albumListMax {
			break
		}
	}
	return out, nil
}

// GetAlbumsByGenre returns every album of one genre.
func (c *Client) GetAlbumsByGenre(genre string) ([]Album, error) {
	var out []Album
	for offset := 0; ; offset += albumListMax {
		extra := url.Values{}
		extra.Set("type", "byGenre")
		extra.Set("genre", genre)
		extra.Set("size", fmt.Sprintf("%d", albumListMax))
		extra.Set("offset", fmt.Sprintf("%d", offset))
		page, err := c.getAlbumList2(extra)
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) < albumListMax {
			break
		}
	}
	return out, nil
}

// GetAlbumsByYear returns every album with a year in the range from..to.
func (c *Client) GetAlbumsByYear(from, to int) ([]Album, error) {
	var out []Album
	for offset := 0; ; offset += albumListMax {
		extra := url.Values{}
		extra.Set("type", "byYear")
		extra.Set("fromYear", fmt.Sprintf("%d", from))
		extra.Set("toYear", fmt.Sprintf("%d", to))
		extra.Set("size", fmt.Sprintf("%d", albumListMax))
		extra.Set("offset", fmt.Sprintf("%d", offset))
		page, err := c.getAlbumList2(extra)
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) < albumListMax {
			break
		}
	}
	return out, nil
}

// SearchSongs returns songs that match the query. An empty query returns a
// broad set. count limits the number of songs.
func (c *Client) SearchSongs(query string, count int) ([]Song, error) {
	extra := url.Values{}
	extra.Set("query", query)
	extra.Set("songCount", fmt.Sprintf("%d", count))
	extra.Set("artistCount", "0")
	extra.Set("albumCount", "0")
	var r struct {
		Response struct {
			baseResponse
			SearchResult3 struct {
				Song []Song `json:"song"`
			} `json:"searchResult3"`
		} `json:"subsonic-response"`
	}
	if err := c.get("search3", extra, &r); err != nil {
		return nil, err
	}
	if r.Response.Error != nil {
		return nil, r.Response.Error
	}
	return r.Response.SearchResult3.Song, nil
}

// Playlist is one saved playlist.
type Playlist struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SongCount int    `json:"songCount"`
	Duration  int    `json:"duration"`
	Songs     []Song `json:"entry"`
}

// GetPlaylists returns all playlists the user may play.
func (c *Client) GetPlaylists() ([]Playlist, error) {
	var r struct {
		Response struct {
			baseResponse
			Playlists struct {
				Playlist []Playlist `json:"playlist"`
			} `json:"playlists"`
		} `json:"subsonic-response"`
	}
	if err := c.get("getPlaylists", nil, &r); err != nil {
		return nil, err
	}
	if r.Response.Error != nil {
		return nil, r.Response.Error
	}
	return r.Response.Playlists.Playlist, nil
}

// GetPlaylist returns one playlist and its songs.
func (c *Client) GetPlaylist(id string) (Playlist, error) {
	extra := url.Values{}
	extra.Set("id", id)
	var r struct {
		Response struct {
			baseResponse
			Playlist Playlist `json:"playlist"`
		} `json:"subsonic-response"`
	}
	if err := c.get("getPlaylist", extra, &r); err != nil {
		return Playlist{}, err
	}
	if r.Response.Error != nil {
		return Playlist{}, r.Response.Error
	}
	return r.Response.Playlist, nil
}

// CreatePlaylist creates a playlist with the given name and song IDs.
func (c *Client) CreatePlaylist(name string, songIDs []string) error {
	extra := url.Values{}
	extra.Set("name", name)
	for _, id := range songIDs {
		extra.Add("songId", id)
	}
	var r struct {
		Response baseResponse `json:"subsonic-response"`
	}
	if err := c.get("createPlaylist", extra, &r); err != nil {
		return err
	}
	if r.Response.Error != nil {
		return r.Response.Error
	}
	return nil
}

// DeletePlaylist deletes a saved playlist.
func (c *Client) DeletePlaylist(id string) error {
	extra := url.Values{}
	extra.Set("id", id)
	var r struct {
		Response baseResponse `json:"subsonic-response"`
	}
	if err := c.get("deletePlaylist", extra, &r); err != nil {
		return err
	}
	if r.Response.Error != nil {
		return r.Response.Error
	}
	return nil
}

// UpdatePlaylistSongs replaces the songs of an existing playlist. It uses
// createPlaylist with a playlistId, which Subsonic treats as an update.
func (c *Client) UpdatePlaylistSongs(id string, songIDs []string) error {
	extra := url.Values{}
	extra.Set("playlistId", id)
	for _, sid := range songIDs {
		extra.Add("songId", sid)
	}
	var r struct {
		Response baseResponse `json:"subsonic-response"`
	}
	if err := c.get("createPlaylist", extra, &r); err != nil {
		return err
	}
	if r.Response.Error != nil {
		return r.Response.Error
	}
	return nil
}

// StartScan asks the server to scan the library. Navidrome imports new
// playlist files, including smart-playlist (.nsp) files, during a scan.
// This runs an incremental scan. The server may reject the call for a
// non-admin user; the caller treats that as a soft failure.
func (c *Client) StartScan() error {
	var r struct {
		Response baseResponse `json:"subsonic-response"`
	}
	if err := c.get("startScan", nil, &r); err != nil {
		return err
	}
	if r.Response.Error != nil {
		return r.Response.Error
	}
	return nil
}

// ScanStatus reports whether the server scans the library now.
type ScanStatus struct {
	Scanning bool `json:"scanning"`
	Count    int  `json:"count"`
}

// GetScanStatus returns the current library scan status. It takes no
// parameters and is available to normal users, unlike StartScan.
func (c *Client) GetScanStatus() (ScanStatus, error) {
	var r struct {
		Response struct {
			baseResponse
			ScanStatus ScanStatus `json:"scanStatus"`
		} `json:"subsonic-response"`
	}
	if err := c.get("getScanStatus", nil, &r); err != nil {
		return ScanStatus{}, err
	}
	if r.Response.Error != nil {
		return ScanStatus{}, r.Response.Error
	}
	return r.Response.ScanStatus, nil
}

// GetSong returns the details for one song.
func (c *Client) GetSong(id string) (Song, error) {
	extra := url.Values{}
	extra.Set("id", id)
	var r struct {
		Response struct {
			baseResponse
			Song Song `json:"song"`
		} `json:"subsonic-response"`
	}
	if err := c.get("getSong", extra, &r); err != nil {
		return Song{}, err
	}
	if r.Response.Error != nil {
		return Song{}, r.Response.Error
	}
	return r.Response.Song, nil
}

// Star stars one song. The server marks the song as a favorite.
func (c *Client) Star(id string) error {
	extra := url.Values{}
	extra.Set("id", id)
	var r struct {
		Response baseResponse `json:"subsonic-response"`
	}
	if err := c.get("star", extra, &r); err != nil {
		return err
	}
	if r.Response.Error != nil {
		return r.Response.Error
	}
	return nil
}

// SetRating sets the user rating of one song. The rating is 0 to 5. A
// rating of 0 clears the rating. A rating of 1 marks the song as
// disliked. The server persists the value.
func (c *Client) SetRating(id string, rating int) error {
	if rating < 0 {
		rating = 0
	}
	if rating > 5 {
		rating = 5
	}
	extra := url.Values{}
	extra.Set("id", id)
	extra.Set("rating", strconv.Itoa(rating))
	var r struct {
		Response baseResponse `json:"subsonic-response"`
	}
	if err := c.get("setRating", extra, &r); err != nil {
		return err
	}
	if r.Response.Error != nil {
		return r.Response.Error
	}
	return nil
}

// Unstar removes the star from one song.
func (c *Client) Unstar(id string) error {
	extra := url.Values{}
	extra.Set("id", id)
	var r struct {
		Response baseResponse `json:"subsonic-response"`
	}
	if err := c.get("unstar", extra, &r); err != nil {
		return err
	}
	if r.Response.Error != nil {
		return r.Response.Error
	}
	return nil
}

// PlayQueue is a saved play queue. The indexBasedQueue extension keys the
// current track by index, not by song ID, so a queue may hold duplicates.
type PlayQueue struct {
	// Songs holds the queue entries in order.
	Songs []Song
	// CurrentIndex is the index of the current track in Songs, or -1.
	CurrentIndex int
	// Position is the playback offset of the current track in
	// milliseconds.
	Position int64
}

// SavePlayQueueByIndex saves the play queue to the server using the
// indexBasedQueue extension. currentIndex is the index of the current
// track, or -1 when none. positionMs is the offset of the current track in
// milliseconds. An empty songIDs list clears the saved queue.
func (c *Client) SavePlayQueueByIndex(songIDs []string, currentIndex int, positionMs int64) error {
	extra := url.Values{}
	for _, id := range songIDs {
		extra.Add("id", id)
	}
	if currentIndex >= 0 {
		extra.Set("currentIndex", fmt.Sprintf("%d", currentIndex))
	}
	extra.Set("position", fmt.Sprintf("%d", positionMs))
	var r struct {
		Response baseResponse `json:"subsonic-response"`
	}
	if err := c.get("savePlayQueueByIndex", extra, &r); err != nil {
		return err
	}
	if r.Response.Error != nil {
		return r.Response.Error
	}
	return nil
}

// GetPlayQueueByIndex returns the saved play queue from the server using
// the indexBasedQueue extension. The bool is false when the server has no
// saved queue.
func (c *Client) GetPlayQueueByIndex() (PlayQueue, bool, error) {
	var r struct {
		Response struct {
			baseResponse
			PlayQueue *struct {
				Entry        []Song `json:"entry"`
				CurrentIndex *int   `json:"currentIndex"`
				Position     int64  `json:"position"`
			} `json:"playQueueByIndex"`
		} `json:"subsonic-response"`
	}
	if err := c.get("getPlayQueueByIndex", nil, &r); err != nil {
		return PlayQueue{}, false, err
	}
	if r.Response.Error != nil {
		return PlayQueue{}, false, r.Response.Error
	}
	pq := r.Response.PlayQueue
	if pq == nil || len(pq.Entry) == 0 {
		return PlayQueue{}, false, nil
	}
	idx := -1
	if pq.CurrentIndex != nil {
		idx = *pq.CurrentIndex
	}
	return PlayQueue{Songs: pq.Entry, CurrentIndex: idx, Position: pq.Position}, true, nil
}
