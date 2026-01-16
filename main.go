package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Question struct {
	Question      string   `json:"question"`
	AnswersList   []string `json:"answersList,omitempty"`
	CorrectAnswer any      `json:"correctAnswer"`
}

type Test struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Type        int        `json:"type"`
	IconURL     string     `json:"iconURL"`
	Questions   []Question `json:"questions"`
}

type TestMetadata struct {
	ID          int       `json:"id"`
	CategoryID  int       `json:"category_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Type        int       `json:"type"`
	IconURL     string    `json:"iconURL"`
	CreatedAt   time.Time `json:"created_at"`
	ModifiedAt  time.Time `json:"modified_at"`
}

type FolderInfo struct {
	Category    string `json:"category"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

type CategoryMetadata struct {
	ID          int    `json:"id"`
	ParentID    int    `json:"parent_id"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Path        string `json:"path"`
}

var (
	tests          = make(map[int]*Test)
	testsMetadata  = make(map[int][]TestMetadata)
	categories     = make(map[int]*CategoryMetadata)
	categoryByPath = make(map[string]int)
	nextTestID     = 1
	nextCategoryID = 1
	testsDir       = "./tests"
)

func loadFolderInfo(path string) (*FolderInfo, error) {
	infoPath := filepath.Join(path, "folder_info.json")
	data, err := os.ReadFile(infoPath)
	if err != nil {
		return nil, err
	}

	var info FolderInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}

	return &info, nil
}

func loadCategories(path string, parentID int) error {
	relPath, _ := filepath.Rel(testsDir, path)
	if relPath == "." {
		relPath = ""
	}

	info, err := loadFolderInfo(path)
	if err != nil {
		log.Printf("Warning: No folder_info.json in %s: %v", path, err)
		return nil
	}

	var categoryID int
	if path == testsDir {
		categoryID = 0
		nextCategoryID = 1
	} else {
		categoryID = nextCategoryID
		nextCategoryID++
	}

	category := &CategoryMetadata{
		ID:          categoryID,
		ParentID:    parentID,
		Category:    info.Category,
		Description: info.Description,
		Icon:        info.Icon,
		Path:        relPath,
	}

	categories[categoryID] = category
	categoryByPath[relPath] = categoryID
	testsMetadata[categoryID] = []TestMetadata{}

	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		subPath := filepath.Join(path, entry.Name())
		if err := loadCategories(subPath, categoryID); err != nil {
			log.Printf("Error loading category %s: %v", subPath, err)
		}
	}

	return nil
}

func loadTestsInCategory(path string, categoryID int) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			subPath := filepath.Join(path, entry.Name())
			relPath, _ := filepath.Rel(testsDir, subPath)
			if subCategoryID, exists := categoryByPath[relPath]; exists {
				if err := loadTestsInCategory(subPath, subCategoryID); err != nil {
					log.Printf("Error loading tests in %s: %v", subPath, err)
				}
			}
			continue
		}

		if !strings.HasSuffix(entry.Name(), ".json") || entry.Name() == "folder_info.json" {
			continue
		}

		testPath := filepath.Join(path, entry.Name())
		data, err := os.ReadFile(testPath)
		if err != nil {
			log.Printf("Error reading %s: %v", entry.Name(), err)
			continue
		}

		var test Test
		if err := json.Unmarshal(data, &test); err != nil {
			log.Printf("Error parsing %s: %v", entry.Name(), err)
			continue
		}

		info, err := entry.Info()
		if err != nil {
			log.Printf("Error getting info for %s: %v", entry.Name(), err)
			continue
		}

		metadata := TestMetadata{
			ID:          nextTestID,
			CategoryID:  categoryID,
			Name:        test.Name,
			Description: test.Description,
			Type:        test.Type,
			IconURL:     test.IconURL,
			CreatedAt:   info.ModTime(),
			ModifiedAt:  info.ModTime(),
		}

		tests[nextTestID] = &test
		testsMetadata[categoryID] = append(testsMetadata[categoryID], metadata)
		nextTestID++
	}

	return nil
}

func initializeData() error {
	if err := os.MkdirAll(testsDir, 0755); err != nil {
		return err
	}

	tests = make(map[int]*Test)
	testsMetadata = make(map[int][]TestMetadata)
	categories = make(map[int]*CategoryMetadata)
	categoryByPath = make(map[string]int)
	nextTestID = 1
	nextCategoryID = 1

	if err := loadCategories(testsDir, -1); err != nil {
		return err
	}

	if err := loadTestsInCategory(testsDir, 0); err != nil {
		return err
	}

	log.Printf("Loaded %d categories and %d tests", len(categories), len(tests))
	return nil
}

func getCategoriesHandler(w http.ResponseWriter, r *http.Request) {
	categoryList := make([]CategoryMetadata, 0, len(categories))
	for _, cat := range categories {
		categoryList = append(categoryList, *cat)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(categoryList)
}

func getCategoryByIDHandler(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/categories/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid category ID", http.StatusBadRequest)
		return
	}

	category, exists := categories[id]
	if !exists {
		http.Error(w, "Category not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(category)
}

func getTestsHandler(w http.ResponseWriter, r *http.Request) {
	categoryIDStr := r.URL.Query().Get("category_id")

	if categoryIDStr == "" {
		allTests := []TestMetadata{}
		for _, testList := range testsMetadata {
			allTests = append(allTests, testList...)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(allTests)
		return
	}

	categoryID, err := strconv.Atoi(categoryIDStr)
	if err != nil {
		http.Error(w, "Invalid category ID", http.StatusBadRequest)
		return
	}

	tests, exists := testsMetadata[categoryID]
	if !exists {
		http.Error(w, "Category not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tests)
}

func getTestByIDHandler(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/tests/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid test ID", http.StatusBadRequest)
		return
	}

	test, exists := tests[id]
	if !exists {
		http.Error(w, "Test not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(test)
}

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

func main() {
	if err := initializeData(); err != nil {
		log.Fatalf("Failed to initialize data: %v", err)
	}

	http.HandleFunc("/api/categories", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/categories" {
			getCategoriesHandler(w, r)
		} else {
			getCategoryByIDHandler(w, r)
		}
	}))

	http.HandleFunc("/api/tests", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tests" {
			getTestsHandler(w, r)
		} else {
			getTestByIDHandler(w, r)
		}
	}))

	http.HandleFunc("/api/tests/", corsMiddleware(getTestByIDHandler))

	http.Handle("/", http.FileServer(http.Dir("./static")))

	port := "8080"
	fmt.Printf("Server starting on http://localhost:%s\n", port)
	fmt.Printf("Categories loaded: %d\n", len(categories))
	fmt.Printf("Tests loaded: %d\n", len(tests))
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
