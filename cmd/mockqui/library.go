package main

import "strings"

// The tracker monitor demo: torrents seeded from Demo Atlas (announce host
// atlas.demo) compared against Demo Beacon. Each torrent shows one outcome.
const (
	matrixRemuxHash = "cccccccccccccccccccccccccccccccccccccccc" // Radarr download history: verified
	matrixWebHash   = "dddddddddddddddddddddddddddddddddddddddd" // Radarr parse: inferred, present on Beacon
	duneHash        = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" // verified by imported file name and size
	duneGrabHash    = "7777777777777777777777777777777777777777" // removed from qui after import
	arrivalHash     = "ffffffffffffffffffffffffffffffffffffffff" // verified through its cross-seed
	arrivalSeedHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // cross-seed on orbit.demo that Radarr grabbed
	bannedHash      = "9999999999999999999999999999999999999999" // Radarr lookup: banned group on Beacon
	episodeHash     = "8888888888888888888888888888888888888888" // Sonarr download history, single episode
	unknownHash     = "6666666666666666666666666666666666666666" // not in Sonarr or Radarr
)

type movie struct {
	id, tmdb, year, runtime int
	imdb, title             string
}

var movies = []movie{
	{id: 1, tmdb: 603, year: 1999, runtime: 136, imdb: "tt0133093", title: "The Matrix"},
	{id: 2, tmdb: 438631, year: 2021, runtime: 155, imdb: "tt1160419", title: "Dune"},
	{id: 3, tmdb: 329865, year: 2016, runtime: 116, imdb: "tt2543164", title: "Arrival"},
}

func movieByID(id int) *movie {
	for i := range movies {
		if movies[i].id == id {
			return &movies[i]
		}
	}
	return nil
}

type torrent struct {
	hash, name, tracker, category, path string
	files                               []file
}

type file struct {
	name string
	size int64
}

func (t torrent) size() (n int64) {
	for _, f := range t.files {
		n += f.size
	}
	return n
}

func single(name, ext string, size int64) []file { return []file{{name + ext, size}} }

const (
	duneName    = "Dune.2021.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb"
	arrivalName = "Arrival.2016.1080p.BluRay.DD5.1.x264-SPARKS"
	episodeName = "Game.of.Thrones.S01E01.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb"
)

var inventory = []torrent{
	{matrixRemuxHash, "The.Matrix.1999.2160p.UHD.BluRay.REMUX.HEVC.DV.HDR.TrueHD.7.1.Atmos-FraMeSToR", "https://atlas.demo/announce", "radarr", "/downloads/Demo/movie.mkv", []file{{"movie.mkv", 82123456789}}},
	{matrixWebHash, "The.Matrix.1999.1080p.WEB-DL.DDP5.1.H.264-NTb", "https://atlas.demo/announce", "radarr", "/downloads/Movies/The.Matrix.1999.1080p.WEB-DL.DDP5.1.H.264-NTb.mkv", single("The.Matrix.1999.1080p.WEB-DL.DDP5.1.H.264-NTb", ".mkv", 6432187654)},
	{duneHash, duneName, "https://atlas.demo/announce", "radarr", "/downloads/Movies/" + duneName, single(duneName+"/"+duneName, ".mkv", 7516192768)},
	{arrivalHash, arrivalName, "https://atlas.demo/announce", "movies", "/downloads/Movies/" + arrivalName, single(arrivalName+"/"+arrivalName, ".mkv", 11811160064)},
	{arrivalSeedHash, arrivalName, "https://orbit.demo/announce", "radarr", "/downloads/Movies/" + arrivalName, single(arrivalName+"/"+arrivalName, ".mkv", 11811160064)},
	{bannedHash, "Arrival.2016.720p.BluRay.DD5.1.x264-BADGRP", "https://atlas.demo/announce", "movies", "/downloads/Movies/Arrival.2016.720p.BluRay.DD5.1.x264-BADGRP.mkv", single("Arrival.2016.720p.BluRay.DD5.1.x264-BADGRP", ".mkv", 4831838208)},
	{episodeHash, episodeName, "https://atlas.demo/announce", "tv-sonarr", "/downloads/TV/" + episodeName + ".mkv", single(episodeName, ".mkv", 2147483648)},
	{unknownHash, "Night.of.the.Living.Dead.1968.1080p.BluRay.x264-OLDGRP", "https://atlas.demo/announce", "movies", "/downloads/Movies/Night.of.the.Living.Dead.1968.1080p.BluRay.x264-OLDGRP.mkv", single("Night.of.the.Living.Dead.1968.1080p.BluRay.x264-OLDGRP", ".mkv", 8589934592)},
}

func torrentByHash(hash string) *torrent {
	for i := range inventory {
		if strings.EqualFold(inventory[i].hash, hash) {
			return &inventory[i]
		}
	}
	return nil
}

// listing is a release a demo tracker lists for a monitor fixture title.
type listing struct {
	indexers []int
	title    string
	size     int64
	seeders  int
}

// listings by IMDb ID, for titles outside the original search demo.
var listings = map[string][]listing{
	"tt1160419": {
		{[]int{1, 2}, duneName, 7516192768, 41},
		{[]int{3}, "Dune.2021.REPACK.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb", 7516201472, 18},
	},
	"tt2543164": {
		{[]int{1, 2}, arrivalName, 11811160064, 27},
		{[]int{1}, "Arrival.2016.720p.BluRay.DD5.1.x264-BADGRP", 4831838208, 3},
		{[]int{3}, "Arrival.2016.1080p.BluRay.DTS.x264-DON", 12348030976, 9},
	},
	"tt0944947": {
		{[]int{1}, episodeName, 2147483648, 55},
		{[]int{3}, "Game.of.Thrones.S01.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb", 21474836480, 33},
	},
}

// fixtureListings answers searches for monitor fixture titles by ID or title.
func fixtureListings(imdb string, tmdb int, tvdb, query string, indexer int) ([]listing, bool) {
	key := imdb
	switch {
	case tmdb == 438631 || strings.HasPrefix(strings.ToLower(query), "dune"):
		key = "tt1160419"
	case tmdb == 329865 || strings.HasPrefix(strings.ToLower(query), "arrival"):
		key = "tt2543164"
	case tmdb == 1399 || tvdb == "121361" || strings.HasPrefix(strings.ToLower(query), "game of thrones"):
		key = "tt0944947"
	case strings.HasPrefix(strings.ToLower(query), "night of the living dead"):
		return []listing{}, true
	}
	all, ok := listings[key]
	if !ok {
		return nil, false
	}
	out := []listing{}
	for _, v := range all {
		for _, id := range v.indexers {
			if id == indexer {
				out = append(out, v)
			}
		}
	}
	return out, true
}
