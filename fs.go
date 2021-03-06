package fatfs

import (
	"errors"

	"github.com/unixpickle/essentials"
)

// FS provides all the information needed to perform
// file-system operations.
type FS struct {
	Device     BlockDevice
	BootSector *BootSector

	fatSectors []uint32
}

// NewFS creates a file-system using the block device.
func NewFS(b BlockDevice) (*FS, error) {
	bsData, err := b.ReadSector(0)
	if err != nil {
		return nil, essentials.AddCtx("NewFS", err)
	}
	bs := BootSector(*bsData)
	fs := &FS{Device: b, BootSector: &bs}
	offset := uint32(bs.RsvdSecCnt())
	for i := 0; i < int(bs.NumFATs()); i++ {
		fs.fatSectors = append(fs.fatSectors, offset)
		offset += bs.FatSz32()
	}
	return fs, nil
}

// FormatFS creates a file-system by formatting the block
// device.
//
// If erase is false, then it is assumed that all the data
// on the device was already zeroes.
func FormatFS(b BlockDevice, label string, erase bool) (fs *FS, err error) {
	defer essentials.AddCtxTo("FormatFS", &err)
	bs, err := NewBootSector32(b.NumSectors(), label)
	if err != nil {
		return nil, err
	}

	var sec Sector
	if erase {
		for i := 0; i < int(bs.RsvdSecCnt())+2*int(bs.FatSz32()); i++ {
			if err := b.WriteSector(uint32(i), &sec); err != nil {
				return nil, err
			}
		}
	}

	sec = Sector(*bs)
	if err := b.WriteSector(0, &sec); err != nil {
		return nil, err
	}
	if err := b.WriteSector(1, fsInfoSector()); err != nil {
		return nil, err
	}
	fs, err = NewFS(b)
	if err != nil {
		return nil, err
	}

	// First reserved cluster: 0x0FFFFF<MEDIA>
	// Second reserved cluster: EOC
	// Third cluster: EOC for root directory
	for i := 0; i < 3; i++ {
		if err := fs.WriteFAT(uint32(i), EOF); err != nil {
			return nil, err
		}
	}

	return fs, nil
}

// ClusterSize gets the number of bytes per cluster.
func (f *FS) ClusterSize() int {
	return int(f.BootSector.SecPerClus()) * SectorSize
}

// NumClusters gets the number of data clusters.
func (f *FS) NumClusters() uint32 {
	b := f.BootSector
	numSectors := b.TotSec32() - (b.FatSz32()*uint32(b.NumFATs()) + uint32(b.RsvdSecCnt()))
	return 2 + numSectors/uint32(b.SecPerClus())
}

// ReadFAT reads a FAT entry.
func (f *FS) ReadFAT(dataIndex uint32) (uint32, error) {
	sector, byteIdx := fatIndices(dataIndex)
	block, err := f.Device.ReadSector(f.fatSectors[0] + sector)
	if err != nil {
		return 0, essentials.AddCtx("ReadFAT", err)
	}
	return Endian.Uint32(block[byteIdx:byteIdx+4]) & 0x0fffffff, nil
}

// WriteFAT writes a FAT entry.
func (f *FS) WriteFAT(dataIndex uint32, contents uint32) error {
	sector, byteIdx := fatIndices(dataIndex)
	for _, sectorOffset := range f.fatSectors {
		block, err := f.Device.ReadSector(sector + sectorOffset)
		if err != nil {
			return essentials.AddCtx("WriteFAT", err)
		}
		oldContents := Endian.Uint32(block[byteIdx : byteIdx+4])
		newContents := (contents & 0x0fffffff) | (oldContents & 0xf0000000)
		Endian.PutUint32(block[byteIdx:byteIdx+4], newContents)
		err = f.Device.WriteSector(sector+sectorOffset, block)
		if err != nil {
			return essentials.AddCtx("WriteFAT", err)
		}
	}
	return nil
}

// Alloc allocates a cluster and marks it with an EOF in
// the FAT.
func (f *FS) Alloc() (dataIndex uint32, err error) {
	defer essentials.AddCtxTo("Alloc", &err)
	for i := uint32(0); i < f.BootSector.FatSz32(); i++ {
		block, err := f.Device.ReadSector(i + f.fatSectors[0])
		if err != nil {
			return 0, err
		}
		for j := 0; j < 128; j++ {
			clusterIdx := uint32(j) + i*128
			if clusterIdx < 2 || clusterIdx >= f.NumClusters() {
				continue
			}
			contents := Endian.Uint32(block[j*4:(j+1)*4]) & 0x0fffffff
			if contents == 0 {
				return clusterIdx, f.WriteFAT(clusterIdx, EOF)
			}
		}
	}
	return 0, errors.New("no free clusters")
}

func fatIndices(dataIndex uint32) (uint32, int) {
	sector := dataIndex / 128
	sectorIdx := dataIndex % 128
	return sector, int(sectorIdx) * 4
}

func fsInfoSector() *Sector {
	var res Sector
	Endian.PutUint32(res[0:4], 0x41615252)
	Endian.PutUint32(res[488:492], 0xffffffff)
	Endian.PutUint32(res[492:496], 0xffffffff)
	Endian.PutUint32(res[508:], 0xAA550000)
	return &res
}
