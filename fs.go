package fatfs

import "github.com/unixpickle/essentials"

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

func (f *FS) readFAT(dataIndex uint32) (uint32, error) {
	sector, byteIdx := fatIndices(dataIndex)
	block, err := f.Device.ReadSector(f.fatSectors[0] + sector)
	if err != nil {
		return 0, essentials.AddCtx("ReadFAT", err)
	}
	return Endian.Uint32(block[byteIdx : byteIdx+4]), nil
}

func (f *FS) writeFAT(dataIndex uint32, contents uint32) error {
	sector, byteIdx := fatIndices(dataIndex)
	for _, sectorOffset := range f.fatSectors {
		block, err := f.Device.ReadSector(sector + sectorOffset)
		if err != nil {
			return essentials.AddCtx("WriteFAT", err)
		}
		Endian.PutUint32(block[byteIdx:byteIdx+4], contents)
		err = f.Device.WriteSector(sector+sectorOffset, block)
		if err != nil {
			return essentials.AddCtx("WriteFAT", err)
		}
	}
	return nil
}