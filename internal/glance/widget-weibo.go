package glance

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

var weiboWidgetTemplate = mustParseTemplate("weibo.html", "widget-base.html")

type weiboWidget struct {
	widgetBase `yaml:",inline"`
	
	// 配置参数
	ShowCount     int    `yaml:"show-count"`
	Limit         int    `yaml:"limit"`
	Category      string `yaml:"category"`
	RefreshInterval int   `yaml:"refresh-interval"`
	
	// 内部数据
	HotSearches   []struct {
		weiboHotSearchItem
		URL string
	} `yaml:"-"`
	LastUpdated   time.Time            `yaml:"-"`
}

// 微博热搜项结构
type weiboHotSearchItem struct {
	Icon             string `json:"icon"`               // 图标URL
	IconWidth        int    `json:"icon_width"`         // 图标宽度
	IconHeight       int    `json:"icon_height"`        // 图标高度
	Emoticon         string `json:"emoticon"`           // 表情
	Num              int64  `json:"num"`                // 热度值
	Note             string `json:"note"`               // 备注
	RealPos          int    `json:"realpos"`            // 实际排名
	LabelName        string `json:"label_name"`         // 标签名称
	SmallIconDesc    string `json:"small_icon_desc"`    // 小图标描述
	SmallIconDescColor string `json:"small_icon_desc_color"` // 小图标描述颜色
	Flag             int    `json:"flag"`               // 标志
	IconDesc         string `json:"icon_desc"`          // 图标描述
	IconDescColor    string `json:"icon_desc_color"`    // 图标描述颜色
	WordScheme       string `json:"word_scheme"`        // 带格式的关键词
	TopicFlag        int    `json:"topic_flag"`         // 话题标志
	Word             string `json:"word"`               // 关键词
	Rank             int    `json:"rank"`               // 排名
	FlagDesc         string `json:"flag_desc"`          // 标志描述
	Monitors         map[string]interface{} `json:"monitors,omitempty"` // 监控器
	DotIcon          int    `json:"dot_icon,omitempty"` // 点图标
	IsAd             int    `json:"is_ad,omitempty"`    // 是否广告
	IconType         string `json:"icon_type,omitempty"` // 图标类型
	ID               int    `json:"id,omitempty"`       // ID
}

// 微博API响应结构
type weiboAPIResponse struct {
	OK    int `json:"ok"`
	Data  struct {
		Realtime []weiboHotSearchItem `json:"realtime"`
		Hotgovs  []weiboHotSearchItem `json:"hotgovs"`
		Hotgov   struct {
			SmallIconDesc     string `json:"small_icon_desc"`
			SmallIconDescColor string `json:"small_icon_desc_color"`
			IconHeight        int    `json:"icon_height"`
			IsHot            int    `json:"is_hot"`
			Pos              int    `json:"pos"`
			TopicFlag        int    `json:"topic_flag"`
			Word             string `json:"word"`
			IsGov            int    `json:"is_gov"`
			Note             string `json:"note"`
			Name             string `json:"name"`
			URL              string `json:"url"`
			Flag             int    `json:"flag"`
			IconWidth        int    `json:"icon_width"`
			Stime            int64  `json:"stime"`
			IconDesc         string `json:"icon_desc"`
			Icon             string `json:"icon"`
			IconDescColor    string `json:"icon_desc_color"`
			Mid              string `json:"mid"`
		} `json:"hotgov"`
		Logs struct {
			ActCode int    `json:"act_code"`
			Ext     string `json:"ext"`
		} `json:"logs"`
	} `json:"data"`
}

func (widget *weiboWidget) initialize() error {
	widget.withTitle("Weibo HotSearch").withCacheDuration(30 * time.Minute)

	// 优先使用limit字段，如果未设置则使用show-count
	if widget.Limit > 0 {
		widget.ShowCount = widget.Limit
	}
	
	if widget.ShowCount <= 0 {
		widget.ShowCount = 10
	}
	if widget.ShowCount > 50 {
		widget.ShowCount = 50
	}
	
	if widget.RefreshInterval <= 0 {
		widget.RefreshInterval = 30 // 默认30分钟
	}

	// 设置缓存时间
	widget.withCacheDuration(time.Duration(widget.RefreshInterval) * time.Minute)
	
	// 设置内容可用，确保Widget可以正常显示
	widget.ContentAvailable = true

	return nil
}

func (widget *weiboWidget) update(ctx context.Context) {
	// 获取微博热搜数据
	hotSearches, err := widget.fetchWeiboHotSearch(ctx)
	if err != nil {
		widget.withError(err).scheduleEarlyUpdate()
		return
	}

	widget.HotSearches = hotSearches
	widget.LastUpdated = time.Now()
}

func (widget *weiboWidget) Render() template.HTML {
	return widget.renderTemplate(widget, weiboWidgetTemplate)
}

// 获取微博热搜数据
func (widget *weiboWidget) fetchWeiboHotSearch(ctx context.Context) ([]struct {
	weiboHotSearchItem
	URL string
}, error) {
	// 使用第三方API获取微博热搜数据
	apiURL := "https://weibo.com/ajax/side/hotSearch"
	
	// 创建HTTP请求
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}
	
	// 设置请求头，模拟浏览器访问
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Referer", "https://weibo.com")
	
	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	
	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API请求失败，状态码: %d", resp.StatusCode)
	}
	
	// 读取响应内容
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应内容失败: %v", err)
	}
	
	// 解析JSON响应
	var apiResponse weiboAPIResponse
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return nil, fmt.Errorf("解析JSON响应失败: %v", err)
	}
	
	// 检查API响应状态
	if apiResponse.OK != 1 {
		return nil, fmt.Errorf("API返回错误状态: ok=%d", apiResponse.OK)
	}
	
	// 合并实时热搜和政府热搜
	var allItems []weiboHotSearchItem
	allItems = append(allItems, apiResponse.Data.Realtime...)
	allItems = append(allItems, apiResponse.Data.Hotgovs...)
	
	// 过滤掉空数据
	var filteredHotSearches []weiboHotSearchItem
	for _, item := range allItems {
		if item.Word != "" {
			// 如果指定了类别过滤
			if widget.Category != "" && item.LabelName != widget.Category {
				continue
			}
			filteredHotSearches = append(filteredHotSearches, item)
		}
	}
	
	// 应用限制数量
	if widget.ShowCount > 0 && len(filteredHotSearches) > widget.ShowCount {
		filteredHotSearches = filteredHotSearches[:widget.ShowCount]
	}
	
	// 为模板添加URL字段
	var hotSearchesWithUrl []struct {
		weiboHotSearchItem
		URL string
	}
	for _, item := range filteredHotSearches {
		searchURL := fmt.Sprintf("https://s.weibo.com/weibo?q=%s", url.QueryEscape(item.WordScheme))
		hotSearchesWithUrl = append(hotSearchesWithUrl, struct {
			weiboHotSearchItem
			URL string
		}{
			weiboHotSearchItem: item,
			URL:               searchURL,
		})
	}
	
	return hotSearchesWithUrl, nil
}

// 格式化热度值
func (item *weiboHotSearchItem) FormattedHotValue() string {
	if item.Num >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(item.Num)/1000000)
	} else if item.Num >= 1000 {
		return fmt.Sprintf("%.1fK", float64(item.Num)/1000)
	}
	return strconv.FormatInt(item.Num, 10)
}

// 获取类别显示名称
func (item *weiboHotSearchItem) CategoryDisplayName() string {
	categoryMap := map[string]string{
		"娱乐":   "娱",
		"社会":   "社",
		"科技":   "科",
		"体育":   "体",
		"财经":   "财",
		"热点":   "热",
		"军事":   "军",
		"国际":   "国",
		"历史":   "史",
		"美食":   "食",
	}
	
	if name, ok := categoryMap[item.LabelName]; ok {
		return name
	}
	return item.LabelName
}