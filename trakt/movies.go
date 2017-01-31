package trakt

import (
	"fmt"
	"path"
	"errors"
	"strconv"
	"strings"
	"math/rand"

	"github.com/jmcvetta/napping"
	"github.com/scakemyer/quasar/config"
	"github.com/scakemyer/quasar/cache"
	"github.com/scakemyer/quasar/tmdb"
	"github.com/scakemyer/quasar/xbmc"
)

// Fill fanart from TMDB
func setFanart(movie *Movie) *Movie {
	if movie.IDs == nil || movie.IDs.TMDB == 0 {
		return movie
	}
	tmdbImages := tmdb.GetImages(movie.IDs.TMDB)
	if tmdbImages == nil {
		return movie
	}
	if movie.Images == nil {
		movie.Images = &Images{}
		movie.Images.Poster = &Sizes{}
		movie.Images.Thumbnail = &Sizes{}
		movie.Images.FanArt = &Sizes{}
		movie.Images.Banner = &Sizes{}
	}
	if len(tmdbImages.Posters) > 0 {
		posterImage := tmdb.ImageURL(tmdbImages.Posters[0].FilePath, "w500")
		for _, image := range tmdbImages.Posters {
			if image.ISO_639_1 == config.Get().Language {
				posterImage = tmdb.ImageURL(image.FilePath, "w500")
			}
		}
		movie.Images.Poster.Full = posterImage
		movie.Images.Thumbnail.Full = posterImage
	}
	if len(tmdbImages.Backdrops) > 0 {
		backdropImage := tmdb.ImageURL(tmdbImages.Backdrops[0].FilePath, "w1280")
		for _, image := range tmdbImages.Backdrops {
			if image.ISO_639_1 == config.Get().Language {
				backdropImage = tmdb.ImageURL(image.FilePath, "w1280")
			}
		}
		movie.Images.FanArt.Full = backdropImage
		movie.Images.Banner.Full = backdropImage
	}
	return movie
}

func setFanarts(movies []*Movies) []*Movies {
	for i, movie := range movies {
		movies[i].Movie = setFanart(movie.Movie)
	}
	return movies
}

func setCalendarFanarts(movies []*CalendarMovie) []*CalendarMovie {
	for i, movie := range movies {
		movies[i].Movie = setFanart(movie.Movie)
	}
	return movies
}

func GetMovie(Id string) (movie *Movie) {
	endPoint := fmt.Sprintf("movies/%s", Id)

	params := napping.Params{
		"extended": "full,images",
	}.AsUrlValues()

	cacheStore := cache.NewFileStore(path.Join(config.Get().ProfilePath, "cache"))
	key := fmt.Sprintf("com.trakt.movie.%s", Id)
	if err := cacheStore.Get(key, &movie); err != nil {
		resp, err := Get(endPoint, params)

		if err != nil {
			log.Error(err)
			xbmc.Notify("Quasar", fmt.Sprintf("Failed getting Trakt movie (%s), check your logs.", Id), config.AddonIcon())
		}

		if err := resp.Unmarshal(&movie); err != nil {
			log.Warning(err)
		}

		movie = setFanart(movie)

		cacheStore.Set(key, movie, cacheExpiration)
	}

	return
}

func SearchMovies(query string, page string) (movies []*Movies, err error) {
	endPoint := "search"

	params := napping.Params{
		"page": page,
		"limit": strconv.Itoa(config.Get().ResultsPerPage),
		"query": query,
		"extended": "full,images",
	}.AsUrlValues()

	resp, err := Get(endPoint, params)

	if err != nil {
		return
	} else if resp.Status() != 200 {
		return movies, errors.New(fmt.Sprintf("Bad status searching Trakt movies: %d", resp.Status()))
	}

  // TODO use response headers for pagination limits:
  // X-Pagination-Page-Count:10
  // X-Pagination-Item-Count:100

	if err := resp.Unmarshal(&movies); err != nil {
		log.Warning(err)
	}

	if page != "0" {
		movies = setFanarts(movies)
	}

	return
}

func TopMovies(topCategory string, page string) (movies []*Movies, err error) {
	endPoint := "movies/" + topCategory

	resultsPerPage := config.Get().ResultsPerPage
	limit := resultsPerPage * PagesAtOnce
	pageInt, err := strconv.Atoi(page)
	if err != nil {
		return
	}
	page = strconv.Itoa((pageInt - 1) * resultsPerPage / limit + 1)
	params := napping.Params{
		"page": page,
		"limit": strconv.Itoa(limit),
		"extended": "full,images",
	}.AsUrlValues()

	cacheStore := cache.NewFileStore(path.Join(config.Get().ProfilePath, "cache"))
	key := fmt.Sprintf("com.trakt.movies.%s.%s", topCategory, page)
	if err := cacheStore.Get(key, &movies); err != nil {
		resp, err := Get(endPoint, params)

		if err != nil {
			return movies, err
		} else if resp.Status() != 200 {
			return movies, errors.New(fmt.Sprintf("Bad status getting top %s Trakt shows: %d", topCategory, resp.Status()))
		}

		if topCategory == "popular" {
			var movieList []*Movie
			if err := resp.Unmarshal(&movieList); err != nil {
				log.Warning(err)
			}

		  movieListing := make([]*Movies, 0)
		  for _, movie := range movieList {
				movieItem := Movies{
		      Movie: movie,
		    }
		    movieListing = append(movieListing, &movieItem)
		  }
			movies = movieListing
		} else {
			if err := resp.Unmarshal(&movies); err != nil {
				log.Warning(err)
			}
		}

		if page != "0" {
			movies = setFanarts(movies)
		}

		cacheStore.Set(key, movies, recentExpiration)
	}

	return
}

func WatchlistMovies() (movies []*Movies, err error) {
	if err := Authorized(); err != nil {
		return movies, err
	}

	endPoint := "sync/watchlist/movies"

	params := napping.Params{
		"extended": "full,images",
	}.AsUrlValues()

	cacheStore := cache.NewFileStore(path.Join(config.Get().ProfilePath, "cache"))
	key := "com.trakt.movies.watchlist"
	if err := cacheStore.Get(key, &movies); err != nil {
		resp, err := GetWithAuth(endPoint, params)

		if err != nil {
			return movies, err
		} else if resp.Status() != 200 {
			return movies, errors.New(fmt.Sprintf("Bad status getting Trakt watchlist for movies: %d", resp.Status()))
		}

		var watchlist []*WatchlistMovie
		if err := resp.Unmarshal(&watchlist); err != nil {
			log.Warning(err)
		}

		movieListing := make([]*Movies, 0)
		for _, movie := range watchlist {
			movieItem := Movies{
				Movie: movie.Movie,
			}
			movieListing = append(movieListing, &movieItem)
		}
		movies = movieListing

		movies = setFanarts(movies)

		cacheStore.Set(key, movies, userlistExpiration)
	}

	return
}

func CollectionMovies() (movies []*Movies, err error) {
	if err := Authorized(); err != nil {
		return movies, err
	}

	endPoint := "sync/collection/movies"

	params := napping.Params{
		"extended": "full,images",
	}.AsUrlValues()

	cacheStore := cache.NewFileStore(path.Join(config.Get().ProfilePath, "cache"))
	key := "com.trakt.movies.collection"
	if err := cacheStore.Get(key, &movies); err != nil {
		resp, err := GetWithAuth(endPoint, params)

		if err != nil {
			return movies, err
		} else if resp.Status() != 200 {
			return movies, errors.New(fmt.Sprintf("Bad status getting Trakt collection for movies: %d", resp.Status()))
		}

		var collection []*CollectionMovie
		resp.Unmarshal(&collection)

		movieListing := make([]*Movies, 0)
		for _, movie := range collection {
			movieItem := Movies{
				Movie: movie.Movie,
			}
			movieListing = append(movieListing, &movieItem)
		}
		movies = movieListing

		movies = setFanarts(movies)

		cacheStore.Set(key, movies, userlistExpiration)
	}

	return movies, err
}

func Userlists() (lists []*List) {
	endPoint := fmt.Sprintf("users/%s/lists", config.Get().TraktUsername)

	params := napping.Params{}.AsUrlValues()

	var resp *napping.Response
	var err error

	if erra := Authorized(); erra != nil {
		resp, err = Get(endPoint, params)
	} else {
		resp, err = GetWithAuth(endPoint, params)
	}

	if err != nil || resp.Status() != 200 {
		return lists
	}

	if err := resp.Unmarshal(&lists); err != nil {
		log.Warning(err)
	}

	return lists
}

func ListItemsMovies(listId string, page string) (movies []*Movies, err error) {
	endPoint := fmt.Sprintf("users/%s/lists/%s/items/movies", config.Get().TraktUsername, listId)

	params := napping.Params{}.AsUrlValues()

	if page != "0" {
		params = napping.Params{
			"page": page,
			"limit": strconv.Itoa(config.Get().ResultsPerPage),
			"extended": "full,images",
		}.AsUrlValues()
	}

	var resp *napping.Response

	if erra := Authorized(); erra != nil {
		resp, err = Get(endPoint, params)
	} else {
		resp, err = GetWithAuth(endPoint, params)
	}

	if err != nil || resp.Status() != 200 {
		return movies, err
	}

	var list []*ListItem
	if err = resp.Unmarshal(&list); err != nil {
		log.Warning(err)
	}

	movieListing := make([]*Movies, 0)
	for _, movie := range list {
		if movie.Movie == nil {
			continue
		}
		movieItem := Movies{
			Movie: movie.Movie,
		}
		movieListing = append(movieListing, &movieItem)
	}
	movies = movieListing

	if page != "0" {
		movies = setFanarts(movies)
	}

	return movies, err
}

func CalendarMovies(endPoint string, page string) (movies []*CalendarMovie, err error) {
	resultsPerPage := config.Get().ResultsPerPage
	limit := resultsPerPage * PagesAtOnce
	pageInt, err := strconv.Atoi(page)
	if err != nil {
		return
	}
	page = strconv.Itoa((pageInt - 1) * resultsPerPage / limit + 1)
	params := napping.Params{
		"page": page,
		"limit": strconv.Itoa(limit),
		"extended": "full,images",
	}.AsUrlValues()

	cacheStore := cache.NewFileStore(path.Join(config.Get().ProfilePath, "cache"))
	key := fmt.Sprintf("com.trakt.mymovies.%s.%s", strings.Replace(endPoint, "/", ".", -1), page)
	if err := cacheStore.Get(key, &movies); err != nil {
		resp, err := GetWithAuth("calendars/" + endPoint, params)

		if err != nil {
			log.Error(err)
			return movies, err
		} else if resp.Status() != 200 {
			log.Warning(resp.Status())
			return movies, errors.New(fmt.Sprintf("Bad status getting %s Trakt movies: %d", endPoint, resp.Status()))
		}

		if err := resp.Unmarshal(&movies); err != nil {
			log.Warning(err)
		}

		if page != "0" {
			movies = setCalendarFanarts(movies)
		}

		cacheStore.Set(key, movies, recentExpiration)
	}

	return
}

func (movie *Movie) ToListItem() *xbmc.ListItem {
	return &xbmc.ListItem{
		Label: movie.Title,
		Info: &xbmc.ListItemInfo{
			Count:         rand.Int(),
			Title:         movie.Title,
			OriginalTitle: movie.Title,
			Year:          movie.Year,
			Genre:         strings.Title(strings.Join(movie.Genres, " / ")),
			Plot:          movie.Overview,
			PlotOutline:   movie.Overview,
			TagLine:       movie.TagLine,
			Rating:        movie.Rating,
			Votes:         strconv.Itoa(movie.Votes),
			Duration:      movie.Runtime * 60,
			MPAA:          movie.Certification,
			Code:          movie.IDs.IMDB,
			IMDBNumber:    movie.IDs.IMDB,
			Trailer:       movie.Trailer,
			DBTYPE:        "movie",
			Mediatype:     "movie",
		},
		Art: &xbmc.ListItemArt{
			Poster: movie.Images.Poster.Full,
			FanArt: movie.Images.FanArt.Full,
			Banner: movie.Images.Banner.Full,
			Thumbnail: movie.Images.Thumbnail.Full,
		},
	}
}
