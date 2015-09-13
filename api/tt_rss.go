package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/urandom/readeef"
	"github.com/urandom/readeef/api/processor"
	"github.com/urandom/readeef/content"
	"github.com/urandom/readeef/content/base/search"
	"github.com/urandom/readeef/content/data"
	"github.com/urandom/webfw"
	"github.com/urandom/webfw/context"
	"github.com/urandom/webfw/util"
)

const (
	TTRSS_API_STATUS_OK  = 0
	TTRSS_API_STATUS_ERR = 1
	TTRSS_VERSION        = "1.8.0"
	TTRSS_API_LEVEL      = 12
	TTRSS_SESSION_ID     = "TTRSS_SESSION_ID"
	TTRSS_USER_NAME      = "TTRSS_USER_NAME"

	TTRSS_FAVORITE_ID = -1
	TTRSS_ALL_ID      = -4
)

type TtRss struct {
	webfw.BasePatternController
	fm *readeef.FeedManager
	sp content.SearchProvider
	ap []ArticleProcessor
}

type ttRssRequest struct {
	Op            string           `json:"op"`
	Sid           string           `json:"sid"`
	Seq           int64            `json:"seq"`
	User          string           `json:"user"`
	Password      string           `json:"password"`
	OutputMode    string           `json:"output_mode"`
	UnreadOnly    bool             `json:"unread_only"`
	Limit         int              `json:"limit"`
	Offset        int              `json:"offset"`
	CatId         int              `json:"cat_id"`
	FeedId        data.FeedId      `json:"feed_id"`
	Skip          int              `json:"skip"`
	IsCat         bool             `json:"is_cat"`
	ShowContent   bool             `json:"show_content"`
	ShowExcerpt   bool             `json:"show_excerpt"`
	ViewMode      string           `json:"view_mode"`
	SinceId       data.ArticleId   `json:"since_id"`
	Sanitize      bool             `json:"sanitize"`
	HasSandbox    bool             `json:"has_sandbox"`
	IncludeHeader bool             `json:"include_header"`
	OrderBy       string           `json:"order_by"`
	Search        string           `json:"search"`
	ArticleIds    []data.ArticleId `json:"article_ids"`
	Mode          int              `json:"mode"`
	Field         int              `json:"field"`
	Data          string           `json:"data"`
	ArticleId     []data.ArticleId `json:"article_id"`
	PrefName      string           `json:"pref_name"`
	FeedUrl       string           `json:"feed_url"`
}

type ttRssResponse struct {
	Seq     int64           `json:"seq"`
	Status  int             `json:"status"`
	Content json.RawMessage `json:"content"`
}

type ttRssGenericContent struct {
	Error     string      `json:"error,omitempty"`
	Level     int         `json:"level,omitempty"`
	ApiLevel  int         `json:"api_level,omitempty"`
	Version   string      `json:"version,omitempty"`
	SessionId string      `json:"session_id,omitempty"`
	Status    interface{} `json:"status,omitempty"`
	Unread    int64       `json:"unread,omitempty"`
	Updated   int64       `json:"updated,omitempty"`
	Value     interface{} `json:"value,omitempty"`
}

type ttRssCountersContent []ttRssCounter

type ttRssCounter struct {
	Id         string `json:"id"`
	Counter    int64  `json:"counter"`
	AuxCounter int64  `json:"auxcounter,omitempty"`
}

type ttRssFeedsContent []ttRssFeed

type ttRssFeed struct {
	Id          data.FeedId `json:"id"`
	Title       string      `json:"title"`
	Unread      int64       `json:"unread"`
	CatId       int         `json:"cat_id"`
	FeedUrl     string      `json:"feed_url,omitempty"`
	LastUpdated int64       `json:"last_updated,omitempty"`
	OrderId     int         `json:"order_id,omitempty"`
}

type ttRssHeadlinesHeaderContent []interface{}
type ttRssHeadlinesContent []ttRssHeadline

type ttRssHeadline struct {
	Id        data.ArticleId `json:"id"`
	Unread    bool           `json:"unread"`
	Marked    bool           `json:"marked"`
	Updated   int64          `json:"updated"`
	IsUpdated bool           `json:"is_updated"`
	Title     string         `json:"title"`
	Link      string         `json:"link"`
	FeedId    data.FeedId    `json:"feed_id"`
	Excerpt   string         `json:"excerpt"`
	Content   string         `json:"content"`
	FeedTitle string         `json:"feed_title"`

	Tags   []string `json:"tags"`
	Labels []string `json:"labels"`
}

type ttRssHeadlinesHeader struct {
	Id      data.FeedId    `json:"id"`
	FirstId data.ArticleId `json:"first_id"`
	IsCat   bool           `json:"is_cat"`
}

type ttRssConfigContent struct {
	IconsDir        string `json:"icons_dir"`
	IconsUrl        string `json:"icons_url"`
	DaemonIsRunning bool   `json:"daemon_is_running"`
	NumFeeds        int    `json:"num_feeds"`
}

type ttRssSubscribeContent struct {
	Status struct {
		Code int `json:"code"`
	} `json:"status"`
}

func NewTtRss(fm *readeef.FeedManager, sp content.SearchProvider, processors []ArticleProcessor) TtRss {
	ap := make([]ArticleProcessor, 0, len(processors))
	for _, p := range processors {
		if _, ok := p.(processor.ProxyHTTP); !ok {
			ap = append(ap, p)
		}
	}

	return TtRss{
		webfw.NewBasePatternController(fmt.Sprintf("/v%d/tt-rss/api/", TTRSS_API_LEVEL), webfw.MethodPost, ""),
		fm, sp, ap,
	}
}

func (controller TtRss) Handler(c context.Context) http.Handler {
	repo := readeef.GetRepo(c)
	logger := webfw.GetLogger(c)
	config := readeef.GetConfig(c)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := ttRssRequest{}
		dec := json.NewDecoder(r.Body)
		sess := webfw.GetSession(c, r)

		resp := ttRssResponse{}

		var err error
		var errType string
		var user content.User
		var con interface{}

		switch {
		default:
			err = dec.Decode(&req)
			if err != nil {
				err = fmt.Errorf("Error decoding JSON request: %v", err)
				break
			}

			resp.Seq = req.Seq

			if req.Op != "login" && req.Op != "isLoggedIn" {
				if id, ok := sess.Get(TTRSS_SESSION_ID); ok && id != "" && id == req.Sid {
					userName, _ := sess.Get(TTRSS_USER_NAME)
					if login, ok := userName.(string); ok {
						user = repo.UserByLogin(data.Login(login))
						if repo.Err() != nil {
							errType = "NOT_LOGGED_IN"
						}
					} else {
						errType = "NOT_LOGGED_IN"
					}
				} else {
					errType = "NOT_LOGGED_IN"
				}
			}

			if errType != "" {
				break
			}

			logger.Debugf("TT-RSS OP: %s\n", req.Op)
			switch req.Op {
			case "getApiLevel":
				con = ttRssGenericContent{Level: TTRSS_API_LEVEL}
			case "getVersion":
				con = ttRssGenericContent{Version: TTRSS_VERSION}
			case "login":
				user = repo.UserByLogin(data.Login(req.User))
				if repo.Err() != nil {
					errType = "LOGIN_ERROR"
					break
				}

				if !user.Authenticate(req.Password, []byte(config.Auth.Secret)) {
					errType = "LOGIN_ERROR"
					break
				}

				sessId := util.UUID()
				sess.Set(TTRSS_SESSION_ID, sessId)
				sess.Set(TTRSS_USER_NAME, req.User)

				con = ttRssGenericContent{
					ApiLevel:  TTRSS_API_LEVEL,
					SessionId: sessId,
				}
			case "logout":
				sess.Delete(TTRSS_SESSION_ID)
				sess.Delete(TTRSS_USER_NAME)

				con = ttRssGenericContent{Status: "OK"}
			case "isLoggedIn":
				if id, ok := sess.Get(TTRSS_SESSION_ID); ok && id != "" {
					con = ttRssGenericContent{Status: true}
				} else {
					con = ttRssGenericContent{Status: false}
				}
			case "getUnread":
				var count int64
				counted := false

				if !req.IsCat {
					feed := user.FeedById(req.FeedId)
					count = feed.Count(data.ArticleCountOptions{UnreadOnly: true})
					if feed.HasErr() {
						err = feed.Err()
						break
					}

					counted = true
				}

				if !counted {
					count = user.Count(data.ArticleCountOptions{UnreadOnly: true})
					if user.HasErr() {
						err = fmt.Errorf("Error getting all unread article ids: %v\n", user.Err())
					}
				}

				con = ttRssGenericContent{Unread: count}
			case "getCounters":
				if req.OutputMode == "" {
					req.OutputMode = "flc"
				}
				cContent := ttRssCountersContent{}

				o := data.ArticleCountOptions{UnreadOnly: true}
				unreadCount := user.Count(o)
				cContent = append(cContent,
					ttRssCounter{Id: "global-unread", Counter: unreadCount})

				feeds := user.AllFeeds()
				cContent = append(cContent,
					ttRssCounter{Id: "subscribed-feeds", Counter: int64(len(feeds))})

				cContent = append(cContent,
					ttRssCounter{Id: strconv.Itoa(TTRSS_FAVORITE_ID),
						Counter:    0,
						AuxCounter: int64(len(user.AllFavoriteArticleIds()))})

				cContent = append(cContent,
					ttRssCounter{Id: strconv.Itoa(TTRSS_ALL_ID),
						Counter:    user.Count(),
						AuxCounter: unreadCount})

				if strings.Contains(req.OutputMode, "f") {
					for _, f := range feeds {
						cContent = append(cContent,
							ttRssCounter{Id: strconv.FormatInt(int64(f.Data().Id), 10), Counter: f.Count(o)},
						)

					}
				}

				if user.HasErr() {
					err = fmt.Errorf("Error getting user counters: %v\n", user.Err())
				}

				con = cContent
			case "getFeeds":
				fContent := ttRssFeedsContent{}

				if req.CatId == TTRSS_FAVORITE_ID || req.CatId == TTRSS_ALL_ID {
					if !req.UnreadOnly {
						fContent = append(fContent, ttRssFeed{
							Id:     TTRSS_FAVORITE_ID,
							Title:  "Starred articles",
							Unread: 0,
							CatId:  TTRSS_FAVORITE_ID,
						})
					}

					unread := user.Count(data.ArticleCountOptions{UnreadOnly: true})

					if unread > 0 || !req.UnreadOnly {
						fContent = append(fContent, ttRssFeed{
							Id:     TTRSS_ALL_ID,
							Title:  "All articles",
							Unread: unread,
							CatId:  TTRSS_FAVORITE_ID,
						})
					}
				}

				if req.CatId == 0 || req.CatId == TTRSS_ALL_ID {
					feeds := user.AllFeeds()
					o := data.ArticleCountOptions{UnreadOnly: true}
					for i := range feeds {
						if req.Limit > 0 {
							if i < req.Offset || i >= req.Limit+req.Offset {
								continue
							}
						}

						d := feeds[i].Data()
						unread := feeds[i].Count(o)

						if unread > 0 || !req.UnreadOnly {
							fContent = append(fContent, ttRssFeed{
								Id:          d.Id,
								Title:       d.Title,
								FeedUrl:     d.Link,
								CatId:       0,
								Unread:      unread,
								LastUpdated: time.Now().Unix(),
								OrderId:     0,
							})
						}
					}
				}

				if user.HasErr() {
					err = fmt.Errorf("Error getting user feeds: %v\n", user.Err())
				}

				con = fContent
			case "getCategories":
				con = ttRssFeedsContent{}
			case "getHeadlines":
				if req.FeedId == 0 {
					errType = "INCORRECT_USAGE"
					break
				}

				limit := req.Limit
				if limit == 0 {
					limit = 200
				}

				var articles []content.UserArticle
				var articleRepo content.ArticleRepo
				var feedTitle string
				firstId := data.ArticleId(0)
				o := data.ArticleQueryOptions{Limit: limit, Offset: req.Skip, UnreadFirst: true}

				if req.FeedId == TTRSS_FAVORITE_ID {
					ttRssSetupSorting(req, user)
					o.FavoriteOnly = true
					articleRepo = user
					feedTitle = "Starred articles"
				} else if req.FeedId == TTRSS_ALL_ID {
					ttRssSetupSorting(req, user)
					articleRepo = user
					feedTitle = "All articles"
				} else if req.FeedId > 0 {
					feed := user.FeedById(req.FeedId)

					ttRssSetupSorting(req, feed)
					articleRepo = feed
					feedTitle = feed.Data().Title
				}

				if req.SinceId > 0 {
					o.AfterId = req.SinceId
				}

				if articleRepo != nil {
					if req.Search != "" {
						if controller.sp != nil {
							if as, ok := articleRepo.(content.ArticleSearch); ok {
								articles = as.Query(req.Search, controller.sp, limit, req.Skip)
							}
						}
					} else {
						var skip bool

						switch req.ViewMode {
						case "all_articles":
						case "unread":
							o.UnreadOnly = true
						default:
							skip = true
						}

						if !skip {
							articles = articleRepo.Articles(o)
						}
					}
				}

				if len(articles) > 0 {
					firstId = articles[0].Data().Id

					if len(controller.ap) > 0 {
						for _, p := range controller.ap {
							articles = p.ProcessArticles(articles)
						}
					}
				}

				if req.IncludeHeader {
					header := ttRssHeadlinesHeader{Id: req.FeedId, FirstId: firstId}
					hContent := ttRssHeadlinesHeaderContent{}

					hContent = append(hContent, header)
					hContent = append(hContent, ttRssHeadlinesFromArticles(req, articles, feedTitle, nil))

					con = hContent
				} else {
					con = ttRssHeadlinesFromArticles(req, articles, feedTitle, nil)
				}
			case "updateArticle":
				articles := user.ArticlesById(req.ArticleIds)
				updateCount := int64(0)

				switch req.Field {
				case 0, 2:
					for _, a := range articles {
						d := a.Data()
						updated := false

						switch req.Field {
						case 0:
							switch req.Mode {
							case 0:
								if d.Favorite {
									updated = true
									d.Favorite = false
								}
							case 1:
								if !d.Favorite {
									updated = true
									d.Favorite = true
								}
							case 2:
								updated = true
								d.Favorite = !d.Favorite
							}
						case 2:
							switch req.Mode {
							case 0:
								if d.Read {
									updated = true
									d.Read = false
								}
							case 1:
								if !d.Read {
									updated = true
									d.Read = true
								}
							case 2:
								updated = true
								d.Read = !d.Read
							}
						}

						if updated {
							a.Data(d)
							a.Update()

							if a.HasErr() {
								err = a.Err()
								break
							}

							updateCount++
						}
					}

					if err != nil {
						break
					}

					con = ttRssGenericContent{Status: "OK", Updated: updateCount}
				}
			case "getArticle":
				articles := user.ArticlesById(req.ArticleId)
				feedTitles := map[data.FeedId]string{}

				for _, a := range articles {
					d := a.Data()
					if _, ok := feedTitles[d.FeedId]; !ok {
						f := repo.FeedById(d.FeedId)
						feedTitles[d.FeedId] = f.Data().Title
					}
				}

				if len(controller.ap) > 0 {
					for _, p := range controller.ap {
						articles = p.ProcessArticles(articles)
					}
				}

				con = ttRssHeadlinesFromArticles(req, articles, "", feedTitles)
			case "getConfig":
				con = ttRssConfigContent{DaemonIsRunning: true, NumFeeds: len(user.AllFeeds())}
			case "updateFeed":
				con = ttRssGenericContent{Status: "OK"}
			case "catchupFeed":
				if !req.IsCat {
					f := user.FeedById(req.FeedId)
					f.ReadState(true, data.ArticleUpdateStateOptions{BeforeDate: time.Now()})

					if f.HasErr() {
						err = f.Err()
						break
					}

					con = ttRssGenericContent{Status: "OK"}
				}
			case "getPref":
				switch req.PrefName {
				case "DEFAULT_UPDATE_INTERVAL":
					con = ttRssGenericContent{Value: int(config.FeedManager.Converted.UpdateInterval.Minutes())}
				case "DEFAULT_ARTICLE_LIMIT":
					con = ttRssGenericContent{Value: 200}
				case "SHOW_CONTENT_PREVIEW":
					con = ttRssGenericContent{Value: true}
				case "HIDE_READ_FEEDS":
					con = ttRssGenericContent{Value: user.Data().ProfileData["unreadOnly"]}
				case "FEEDS_SORT_BY_UNREAD":
					con = ttRssGenericContent{Value: true}
				}
			case "getLabels":
				con = []interface{}{}
			case "setArticleLabel":
				con = ttRssGenericContent{Status: "OK", Updated: 0}
			case "shareToPublished":
				errType = "Publishing failed"
			case "subscribeToFeed":
				f := repo.FeedByLink(req.FeedUrl)
				for _, u := range f.Users() {
					if u.Data().Login == user.Data().Login {
						con = ttRssSubscribeContent{Status: struct {
							Code int `json:"code"`
						}{0}}
						break
					}
				}

				if f.HasErr() {
					err = f.Err()
					break
				}

				f, err := controller.fm.AddFeedByLink(req.FeedUrl)
				if err != nil {
					errType = "INCORRECT_USAGE"
					break
				}

				uf := user.AddFeed(f)
				if uf.HasErr() {
					err = uf.Err()
					break
				}

				con = ttRssSubscribeContent{Status: struct {
					Code int `json:"code"`
				}{1}}
			case "unsubscribeFeed":
				f := user.FeedById(req.FeedId)
				f.Detach()
				users := f.Users()

				if f.HasErr() {
					err = f.Err()
					if err == content.ErrNoContent {
						errType = "FEED_NOT_FOUND"
					}
					break
				}

				if len(users) == 0 {
					controller.fm.RemoveFeed(f)
				}

				con = ttRssGenericContent{Status: "OK"}
			}
		}

		if err == nil && errType == "" {
			resp.Status = TTRSS_API_STATUS_OK
		} else {
			logger.Infof("Error processing TT-RSS API request: %s %v\n", errType, err)
			resp.Status = TTRSS_API_STATUS_ERR
			switch v := con.(type) {
			case ttRssGenericContent:
				v.Error = errType
			}
		}

		var b []byte
		b, err = json.Marshal(con)
		if err == nil {
			resp.Content = json.RawMessage(b)
		}

		b, err = json.Marshal(&resp)

		if err == nil {
			w.Header().Set("Content-Type", "text/json")
			w.Write(b)
		} else {
			logger.Print(fmt.Errorf("TT-RSS error %s: %v", req.Op, err))

			w.WriteHeader(http.StatusInternalServerError)
		}

	})
}

func ttRssSetupSorting(req ttRssRequest, sorting content.ArticleSorting) {
	switch req.OrderBy {
	case "date_reverse":
		sorting.SortingByDate()
		sorting.Order(data.AscendingOrder)
	default:
		sorting.SortingByDate()
		sorting.Order(data.DescendingOrder)
	}
}

func ttRssHeadlinesFromArticles(req ttRssRequest, articles []content.UserArticle, feedTitle string, feedTitles map[data.FeedId]string) (c ttRssHeadlinesContent) {
	for _, a := range articles {
		d := a.Data()
		title := feedTitle
		if feedTitles != nil {
			title = feedTitles[d.FeedId]
		}
		h := ttRssHeadline{
			Id:        d.Id,
			Unread:    !d.Read,
			Marked:    d.Favorite,
			IsUpdated: !d.Read,
			Title:     d.Title,
			Link:      d.Link,
			FeedId:    d.FeedId,
			FeedTitle: title,
		}

		if req.ShowExcerpt {
			excerpt := search.StripTags(d.Description)
			if len(excerpt) > 100 {
				excerpt = excerpt[:100]
			}

			h.Excerpt = excerpt
		}

		if req.ShowContent {
			h.Content = d.Description
		}

		c = append(c, h)
	}
	return
}
