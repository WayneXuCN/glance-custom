package glance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"
)

const (
	defaultFactCacheDuration = 2 * time.Hour
	factAPIURL              = "https://uselessfacts.jsph.pl/api/v2/facts/random"
	aiAPIURL                = "https://api.siliconflow.cn/v1/chat/completions"
)

var randomFactWidgetTemplate = mustParseTemplate("random-fact.html", "widget-base.html")

// RandomFactWidget 配置结构体
type randomFactWidget struct {
	widgetBase `yaml:",inline"`
	
	// API配置
	APIKey      string `yaml:"apikey"`
	Model       string `yaml:"model"`
	APIURL      string `yaml:"apiurl"`
	
	// 内部状态
	client      *http.Client
	CachedData  *randomFactData
	lastUpdate  time.Time
}

// 随机事实数据结构
type randomFactData struct {
	FactID   string `json:"fact_id"`
	FactText string `json:"fact_text"`
	Content  string `json:"content"`
	Source   string `json:"source"`
}

// 原始事实API响应
type rawFactResponse struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Source string `json:"source,omitempty"`
	SourceURL string `json:"source_url,omitempty"`
	Language string `json:"language,omitempty"`
	Permalink string `json:"permalink,omitempty"`
}

// AI API响应
type aiResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// 初始化随机事实Widget
func (widget *randomFactWidget) initialize() error {
	widget.withTitle("Random Fact").withCacheDuration(time.Duration(widget.CustomCacheDuration))

	// 设置默认缓存时间
	if widget.CustomCacheDuration == 0 {
		widget.CustomCacheDuration = durationField(defaultFactCacheDuration)
		widget.withCacheDuration(defaultFactCacheDuration)
	}
	
	// 检查是否配置了AI API参数
	hasAIConfig := widget.APIKey != "" && widget.Model != "" && widget.APIURL != ""
	
	if !hasAIConfig {
		fmt.Printf("AI API not configured, will use raw facts only\n")
	}
	
	// 初始化HTTP客户端
	widget.client = &http.Client{
		Timeout: 30 * time.Second,
	}
	
	return nil
}

// 更新数据
func (widget *randomFactWidget) update(ctx context.Context) {
	// 检查缓存是否有效
	cacheDuration := time.Duration(widget.CustomCacheDuration)
	if widget.CachedData != nil && time.Since(widget.lastUpdate) < cacheDuration {
		return
	}
	
	// 获取原始事实数据
	rawFact, err := widget.fetchRawFact()
	if err != nil {
		fmt.Printf("Error fetching raw fact: %v\n", err)
		widget.withError(err).scheduleEarlyUpdate()
		return
	}
	
	// 检查是否配置了AI API参数
	hasAIConfig := widget.APIKey != "" && widget.Model != "" && widget.APIURL != ""
	
	var processedContent string
	var source string
	
	if hasAIConfig {
		// 获取AI处理后的内容
		processedContent, err = widget.processWithAI(rawFact.Text)
		if err != nil {
			// 如果AI处理失败，使用原始文本
			processedContent = rawFact.Text
		}
		source = widget.extractModelName()
	} else {
		// 没有配置AI API，使用原始文本
		processedContent = rawFact.Text
		source = "uselessfacts.jsph.pl"
	}
	
	// 更新缓存数据
	widget.CachedData = &randomFactData{
		FactID:   rawFact.ID,
		FactText: rawFact.Text,
		Content:  processedContent,
		Source:   source,
	}
	widget.lastUpdate = time.Now()
	widget.scheduleNextUpdate()
}

// 获取原始事实数据
func (widget *randomFactWidget) fetchRawFact() (*rawFactResponse, error) {
	req, err := http.NewRequest("GET", factAPIURL, nil)
	if err != nil {
		return nil, err
	}
	
	resp, err := widget.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status code %d", resp.StatusCode)
	}
	
	var fact rawFactResponse
	if err := json.NewDecoder(resp.Body).Decode(&fact); err != nil {
		return nil, err
	}
	
	return &fact, nil
}

// 使用AI处理事实内容
func (widget *randomFactWidget) processWithAI(text string) (string, error) {
	if widget.APIKey == "" {
		return "", fmt.Errorf("API key not configured")
	}
	
	payload := map[string]interface{}{
		"model": widget.Model,
		"messages": []map[string]string{
			{
				"role": "system",
				"content": `# Role: Random Fact 理解助手
			## Profile
			- language: zh_CN
			- description: 一位专注于帮助用户理解随机趣事实的智能助手，擅长将英文中的冷知识、趣味事实准确翻译并用自然流畅的语言进行解释说明。
			- background: 拥有语言学与科普传播背景，熟悉全球范围内有趣的冷知识，能够快速理解英文句子中的文化或科学背景，并转化为易于理解的中文表达。
			- personality: 专业、耐心、表达清晰，注重细节，风格亲切自然，避免刻板与学术化语言。
			- expertise: 英文到中文翻译、趣味事实解读、科普内容重构、跨文化信息传递
			- target_audience: 对冷知识、趣味事实感兴趣的中文读者，包括学生、科普爱好者、内容创作者等

			## Skills

			1. 语言翻译与本地化
			- 精准翻译：确保英文句子含义完整、语法正确地转化为中文
			- 口语化表达：避免机械直译，使用符合中文语感的自然表达
			- 文化适配：对涉及西方文化元素的内容进行适当语境转化
			- 术语处理：准确处理品牌名、专有名词（如PEZ）并保留其原貌

			2. 信息补充与解释
			- 背景补充：在不偏离原意的前提下，提供简洁的事实背景或常识解释
			- 逻辑衔接：使补充内容与翻译句自然衔接，形成连贯认知
			- 趣味引导：突出“random fact”的趣味性，增强可读性和记忆点
			- 精炼表达：控制补充说明在1-2句话内，避免冗长或信息过载

			## Rules

			1. 基本原则：
			- 忠实原意：翻译必须准确反映原文事实，不得歪曲或添加虚构内容
			- 语言自然：使用标准现代汉语，避免网络用语、俚语或生硬表达
			- 保持简洁：翻译和补充均需简洁明了，重点突出
			- 禁止标题：不得使用“翻译：”“补充：”“注：”等任何形式的标签或前缀

			2. 行为准则：
			- 两行结构：第一行为翻译，第二行为补充说明，严格各占一行
			- 直接输出：无需引导语、问候语或解释性文字，直接呈现结果
			- 事实严谨：补充内容须基于常识或可查证信息，避免主观臆断
			- 风格统一：保持整体语气轻松有趣但不失专业，契合“random fact”特性

			3. 限制条件：
			- 不回答与翻译+解释无关的提问
			- 不进行多句批量处理，每次仅响应一个英文句子
			- 不提供英文原文分析或语法讲解
			- 不使用任何Markdown、代码块或富文本格式

			## Workflows
			- 目标: 准确翻译用户提供的英文random fact，并提供一行自然流畅的中文补充说明
			- 步骤 1: 解析输入英文句子，识别关键事实、主语、宾语及潜在文化背景
			- 步骤 2: 将句子翻译为自然、通顺的中文，保留原意与语气
			- 步骤 3: 根据事实内容，撰写一句简洁、有信息量且具可读性的补充说明
			- 预期结果: 两行纯文本输出，第一行为翻译，第二行为补充，无任何额外内容

			## OutputFormat

			1. 文本输出：
			- format: text
			- structure: 两行纯文本，第一行为翻译，第二行为补充说明
			- style: 简洁、自然、口语化但不失准确，适合大众传播
			- special_requirements: 不使用换行符以外的任何格式控制；禁止添加标点符号作为前缀（如“-”“*”等）

			2. 格式规范：
			- indentation: 无缩进
			- sections: 不分节，仅两行内容
			- highlighting: 无强调格式，纯文字输出

			3. 验证规则：
			- validation: 输出必须为两行，每行至少8个汉字，不超过60字
			- constraints: 第一行必须为翻译，第二行为解释；不可颠倒或合并
			- error_handling: 若输入非完整句子或无法理解，回复“无法处理该输入，请提供一个完整的英文句子。”

			4. 示例说明：
			1. 示例1：
				- 标题: 咖啡味PEZ糖果
				- 格式类型: text
				- 说明: 展示基础翻译与补充说明的自然衔接
				- 示例内容: |
					PEZ糖果甚至还有咖啡味的。
					这种独特的咖啡味PEZ糖果是该品牌众多创意口味之一，旨在为消费者提供意想不到的趣味体验。

			2. 示例2：
				- 标题: 企鹅的膝盖
				- 格式类型: text
				- 说明: 展示科学类冷知识的解释方式
				- 示例内容: |
					Penguins actually have knees.
					企鹅其实是有膝盖的。
					它们的膝盖隐藏在厚厚的羽毛和身体结构中，外表看起来像是腿很短，实则具备完整的膝关节。

			## Initialization
			作为Random Fact 理解助手，你必须遵守上述Rules，按照Workflows执行任务，并按照OutputFormat输出。`,
			},
			{
				"role": "user",
				"content": text,
			},
		},
		"stream": false,
		"max_tokens": 512,
		"response_format": map[string]string{"type": "text"},
	}
	
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	
	req, err := http.NewRequest("POST", widget.APIURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", err
	}
	
	req.Header.Set("Authorization", "Bearer "+widget.APIKey)
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := widget.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("AI API returned status code %d", resp.StatusCode)
	}
	
	var aiResp aiResponse
	if err := json.NewDecoder(resp.Body).Decode(&aiResp); err != nil {
		return "", err
	}
	
	if aiResp.Error != nil {
		return "", fmt.Errorf("AI API error: %s", aiResp.Error.Message)
	}
	
	if len(aiResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in AI response")
	}
	
	return aiResp.Choices[0].Message.Content, nil
}

// 提取模型名称
func (widget *randomFactWidget) extractModelName() string {
	// 从模型路径中提取模型名称，如 "Qwen/Qwen3-8B" -> "Qwen3-8B"
	if len(widget.Model) == 0 {
		return "unknown"
	}
	
	// 如果包含斜杠，取最后一部分
	for i := len(widget.Model) - 1; i >= 0; i-- {
		if widget.Model[i] == '/' {
			return widget.Model[i+1:]
		}
	}
	
	return widget.Model
}

// 渲染Widget
func (widget *randomFactWidget) Render() template.HTML {
	if widget.CachedData == nil {
		fmt.Printf("No cached data available\n")
		widget.ContentAvailable = false
		widget.withError(fmt.Errorf("no data available"))
		return widget.renderTemplate(nil, mustParseTemplate("widget-base.html"))
	}
	
	widget.ContentAvailable = true
	return widget.renderTemplate(widget, randomFactWidgetTemplate)
}

// 设置Widget提供者
func (widget *randomFactWidget) setProviders(providers *widgetProviders) {
	widget.Providers = providers
}

// 设置Widget ID
func (widget *randomFactWidget) setID(id uint64) {
	widget.ID = id
}

// 获取Widget类型
func (widget *randomFactWidget) GetType() string {
	return "random-fact"
}

// 获取Widget ID
func (widget *randomFactWidget) GetID() uint64 {
	return widget.ID
}

// 设置是否隐藏标题
func (widget *randomFactWidget) setHideHeader(value bool) {
	widget.HideHeader = value
}

// 处理HTTP请求
func (widget *randomFactWidget) handleRequest(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}