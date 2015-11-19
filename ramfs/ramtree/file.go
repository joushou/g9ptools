package ramtree

import (
	"errors"
	"sync"
	"time"

	"github.com/joushou/g9p/protocol"
	"github.com/joushou/g9ptools/fileserver"
)

type RAMOpenFile struct {
	offset uint64
	f      *RAMFile
}

func (of *RAMOpenFile) Seek(offset uint64) error {
	if of.f == nil {
		return errors.New("file not open")
	}
	of.f.RLock()
	defer of.f.RUnlock()
	if offset > uint64(len(of.f.content)) {
		return errors.New("seek past length")
	}
	of.offset = uint64(offset)
	of.f.atime = time.Now()
	return nil
}

func (of *RAMOpenFile) Read(p []byte) (int, error) {
	if of.f == nil {
		return 0, errors.New("file not open")
	}
	of.f.RLock()
	defer of.f.RUnlock()
	maxRead := uint64(len(p))
	if maxRead > uint64(len(of.f.content))-of.offset {
		maxRead = uint64(len(of.f.content)) - of.offset
	}

	copy(p, of.f.content[of.offset:maxRead+of.offset])
	of.offset += maxRead
	of.f.atime = time.Now()
	return int(maxRead), nil
}

func (of *RAMOpenFile) Write(p []byte) (int, error) {
	if of.f == nil {
		return 0, errors.New("file not open")
	}

	// TODO(kl): handle append-only
	wlen := uint64(len(p))

	if wlen+of.offset > uint64(len(of.f.content)) {
		b := make([]byte, wlen+of.offset)
		copy(b, of.f.content[:of.offset])
		of.f.content = b
	}

	copy(of.f.content[of.offset:], p)

	of.offset += wlen
	of.f.mtime = time.Now()
	of.f.atime = of.f.mtime
	of.f.version++
	return int(wlen), nil
}

func (of *RAMOpenFile) Close() error {
	of.f.Lock()
	defer of.f.Unlock()
	of.f.opens--
	of.f = nil
	return nil
}

type RAMFile struct {
	sync.RWMutex
	parent fileserver.File
	content     []byte
	id          uint64
	name        string
	user        string
	group       string
	muser       string
	atime       time.Time
	mtime       time.Time
	version     uint32
	permissions protocol.FileMode
	opens       uint
}

func (f *RAMFile) Parent() (fileserver.File, error) {
	return f.parent, nil
}

func (f *RAMFile) Name() (string, error) {
	return f.name, nil
}

func (f *RAMFile) Qid() (protocol.Qid, error) {
	return protocol.Qid{
		Type:    protocol.QTFILE,
		Version: f.version,
		Path:    f.id,
	}, nil
}

func (f *RAMFile) WriteStat(s protocol.Stat) error {
	f.name = s.Name
	f.user = s.UID
	f.group = s.GID
	f.permissions = s.Mode
	f.mtime = time.Now()
	f.atime = f.mtime
	f.version++
	return nil
}

func (f *RAMFile) Stat() (protocol.Stat, error) {
	q, err := f.Qid()
	if err != nil {
		return protocol.Stat{}, err
	}
	n, err := f.Name()
	if err != nil {
		return protocol.Stat{}, err
	}
	return protocol.Stat{
		Qid:    q,
		Mode:   f.permissions,
		Name:   n,
		Length: uint64(len(f.content)),
		UID:    f.user,
		GID:    f.user,
		MUID:   f.user,
		Atime:  uint32(f.atime.Unix()),
		Mtime:  uint32(f.mtime.Unix()),
	}, nil
}

func (f *RAMFile) Open(user string, mode protocol.OpenMode) (fileserver.OpenFile, error) {
	owner := f.user == user
	if !permCheck(owner, f.permissions, mode) {
		return nil, errors.New("access denied")
	}

	f.atime = time.Now()

	f.Lock()
	defer f.Unlock()
	f.opens++

	return &RAMOpenFile{f: f}, nil
}

func (f *RAMFile) IsDir() (bool, error) {
	return false, nil
}

func NewRAMFile(name string, permissions protocol.FileMode, user, group string) *RAMFile {
	return &RAMFile{
		name:        name,
		permissions: permissions,
		user:        user,
		group:       group,
		muser:       user,
		id:          nextID(),
		atime:       time.Now(),
		mtime:       time.Now(),
	}
}