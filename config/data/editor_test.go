package data

import (
	"testing"

	"github.com/junfuchang/superflare/config/model"
)

func TestGetBookmarksDataAsJSON(t *testing.T) {
	categories, bookmarks := GetBookmarksForEditor()
	if len(categories) == 0 || len(bookmarks) == 0 {
		t.Fatal("GetBookmarksForEditor Failed")
	}
}

func TestGetAndUpdateBookmarksFromEditor(t *testing.T) {

	const categories = `1,链接分类1
2,链接分类2
3,链接分类3
4,链接分类4`
	const bookmarks = `1,示例链接,https://link.example.com,[SuperFlare 应用],evernote,链接描述文本
2,示例链接,https://link.example.com,[SuperFlare 应用],FireHydrant,链接描述文本
3,示例链接,https://link.example.com,[SuperFlare 应用],email,链接描述文本
4,示例链接,https://link.example.com,[SuperFlare 应用],MicrosoftOnenote,链接描述文本
5,示例链接,https://link.example.com,[SuperFlare 应用],Robber,
6,示例链接,https://link.example.com,[SuperFlare 应用],EvPlugType1,
7,示例链接,https://link.example.com,[SuperFlare 应用],FileImage,
8,示例链接,https://link.example.com,[SuperFlare 应用],Image,
9,示例链接,https://link.example.com,链接分类1,checkDecagram,
10,示例链接,https://link.example.com,链接分类1,eraser,
11,示例链接,https://link.example.com,链接分类1,mastodon,
12,示例链接,https://link.example.com,链接分类1,alphaACircleOutline,
13,示例链接,https://link.example.com,链接分类1,flask,
14,示例链接,https://link.example.com,链接分类2,sofaOutline,
15,示例链接,https://link.example.com,链接分类2,BowArrow,
16,示例链接,https://link.example.com,链接分类2,messageCog,
17,示例链接,https://link.example.com,链接分类2,alphaRCircleOutline,
18,示例链接,https://link.example.com,链接分类2,cityVariantOutline,
19,示例链接,https://link.example.com,链接分类3,foodCroissant,
20,示例链接,https://link.example.com,链接分类3,KeyboardOutline,
21,示例链接,https://link.example.com,链接分类3,alphaFCircleOutline,
22,示例链接,https://link.example.com,链接分类3,alphaECircleOutline,
23,示例链接,https://link.example.com,链接分类3,alphaYCircleOutline,
24,示例链接,https://link.example.com,链接分类4,musicCircleOutline,
25,示例链接,https://link.example.com,链接分类4,Incognito,
26,示例链接,https://link.example.com,链接分类4,alphaLCircleOutline,
27,示例链接,https://link.example.com,链接分类4,accountSupervisorCircle,
28,示例链接,https://link.example.com,链接分类4,sproutOutline,`

	updated := UpdateBookmarksFromEditor(categories, bookmarks)
	if !updated {
		t.Fatal("UpdateBookmarksFromEditor Failed")
	}

	bookmarkCategories, ok := getCategoriesFromCSV(categories)
	if ok != nil {
		t.Fatal("getCategoriesFromCSV Failed")
	}

	_, _, ok = getBookmarksFromCSV(bookmarks, bookmarkCategories)
	if ok != nil {
		t.Fatal("getBookmarksFromCSV Failed")
	}
}

func TestGetBookmarksFromCSVRecognizesFixedCategory(t *testing.T) {
	const categories = "1,Links"
	const bookmarks = "1,App,https://app.example.com,[SuperFlare \u5e94\u7528],,home,App link\n2,Bookmark,https://bookmark.example.com,Links,,link,Bookmark link"

	bookmarkCategories, err := getCategoriesFromCSV(categories)
	if err != nil {
		t.Fatal("getCategoriesFromCSV Failed")
	}
	favorite, normal, err := getBookmarksFromCSV(bookmarks, bookmarkCategories)
	if err != nil {
		t.Fatal("getBookmarksFromCSV Failed")
	}
	if len(favorite) != 1 || favorite[0].Category != "" {
		t.Fatal("fixed category should be restored as favorite bookmark")
	}
	if len(normal) != 1 || normal[0].Category != "1" {
		t.Fatal("normal category should be restored by category id")
	}
}

func TestGetBookmarksFromCSVParsesLocalURL(t *testing.T) {
	const categories = "1,Links"
	const bookmarks = "1,App,https://app.example.com,http://192.168.1.10:8080,[SuperFlare \u5e94\u7528],,home,App link\n2,Bookmark,https://bookmark.example.com,http://192.168.1.11:8080,Links,Lab,link,Bookmark link"

	bookmarkCategories, err := getCategoriesFromCSV(categories)
	if err != nil {
		t.Fatal("getCategoriesFromCSV Failed")
	}
	favorite, normal, err := getBookmarksFromCSV(bookmarks, bookmarkCategories)
	if err != nil {
		t.Fatal("getBookmarksFromCSV Failed")
	}
	if len(favorite) != 1 || favorite[0].LocalURL != "http://192.168.1.10:8080" {
		t.Fatalf("favorite local URL was not parsed: %#v", favorite)
	}
	if len(normal) != 1 || normal[0].LocalURL != "http://192.168.1.11:8080" || normal[0].Subdir != "Lab" {
		t.Fatalf("normal bookmark local URL/subdir was not parsed: %#v", normal)
	}
}

func TestPropsRemoveAndRestore(t *testing.T) {
	var input []model.Bookmark
	input = append(input, model.Bookmark{Private: true, LocalURL: "http://192.168.1.10"})

	removed := restorePrivateProp(removePrivateProp(input))
	for i := 0; i < len(removed); i++ {
		if removed[i].Private != false {
			t.Fatal("Remove and restore private prop Failed")
		}
		if removed[i].LocalURL != input[i].LocalURL {
			t.Fatal("Remove and restore local URL Failed")
		}
	}
}
