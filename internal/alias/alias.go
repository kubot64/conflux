package alias

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/kubot64/conflux/internal/apperror"
	"github.com/kubot64/conflux/internal/fileutil"
	"github.com/kubot64/conflux/internal/port"
)

// Store は port.AliasStore を実装する。alias.json への読み書きを管理する。
type Store struct {
	path string
}

// NewStore は指定パスに alias.json を管理する Store を生成する。
func NewStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	return &Store{path: path}, nil
}

type aliasRecord struct {
	Name   string `json:"name"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

func (s *Store) load() ([]aliasRecord, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return []aliasRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	var records []aliasRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *Store) save(records []aliasRecord) error {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.AtomicWrite(s.path, data, 0600)
}

// Set はエイリアスを追加または更新する。
func (s *Store) Set(name, target string, t port.AliasType) error {
	records, err := s.load()
	if err != nil {
		return err
	}
	for i, r := range records {
		if r.Name == name {
			records[i].Target = target
			records[i].Type = string(t)
			return s.save(records)
		}
	}
	records = append(records, aliasRecord{Name: name, Target: target, Type: string(t)})
	return s.save(records)
}

// Get は名前でエイリアスを取得する。存在しない場合は KindNotFound エラーを返す。
func (s *Store) Get(name string) (*port.Alias, error) {
	records, err := s.load()
	if err != nil {
		return nil, err
	}
	for _, r := range records {
		if r.Name == name {
			return &port.Alias{Name: r.Name, Target: r.Target, Type: port.AliasType(r.Type)}, nil
		}
	}
	return nil, apperror.New(apperror.KindNotFound, "alias not found: "+name)
}

// List は全エイリアスを返す。
func (s *Store) List() ([]port.Alias, error) {
	records, err := s.load()
	if err != nil {
		return nil, err
	}
	aliases := make([]port.Alias, len(records))
	for i, r := range records {
		aliases[i] = port.Alias{Name: r.Name, Target: r.Target, Type: port.AliasType(r.Type)}
	}
	return aliases, nil
}

// Delete はエイリアスを削除する。存在しない場合は KindNotFound エラーを返す。
func (s *Store) Delete(name string) error {
	records, err := s.load()
	if err != nil {
		return err
	}
	for i, r := range records {
		if r.Name == name {
			records = append(records[:i], records[i+1:]...)
			return s.save(records)
		}
	}
	return apperror.New(apperror.KindNotFound, "alias not found: "+name)
}
