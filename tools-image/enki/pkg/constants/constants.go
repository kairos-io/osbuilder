package constants

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	GrubDefEntry          = "Kairos"
	EfiLabel              = "COS_GRUB"
	ISOLabel              = "COS_LIVE"
	MountBinary           = "/usr/bin/mount"
	EfiFs                 = "vfat"
	IsoRootFile           = "rootfs.squashfs"
	IsoEFIPath            = "/boot/uefi.img"
	BuildImgName          = "elemental"
	EfiBootPath           = "/EFI/BOOT"
	GrubEfiImagex86       = "/usr/share/grub2/x86_64-efi/grub.efi"
	GrubEfiImageArm64     = "/usr/share/grub2/arm64-efi/grub.efi"
	GrubEfiImagex86Dest   = EfiBootPath + "/bootx64.efi"
	GrubEfiImageArm64Dest = EfiBootPath + "/bootaa64.efi"
	GrubCfg               = "grub.cfg"
	GrubPrefixDir         = "/boot/grub2"
	GrubEfiCfg            = "search --no-floppy --file --set=root " + IsoKernelPath +
		"\nset prefix=($root)" + GrubPrefixDir +
		"\nconfigfile $prefix/" + GrubCfg

	GrubFont          = "/usr/share/grub2/unicode.pf2"
	GrubBootHybridImg = "/usr/share/grub2/i386-pc/boot_hybrid.img"
	SyslinuxFiles     = "/usr/share/syslinux/isolinux.bin " +
		"/usr/share/syslinux/menu.c32 " +
		"/usr/share/syslinux/chain.c32 " +
		"/usr/share/syslinux/mboot.c32"
	IsoLoaderPath   = "/boot/x86_64/loader"
	GrubCfgTemplate = `search --no-floppy --file --set=root /boot/kernel                               
	set default=0                                                                   
	set timeout=10                                                                  
	set timeout_style=menu                                                          
	set linux=linux                                                                 
	set initrd=initrd                                                               
	if [ "${grub_cpu}" = "x86_64" -o "${grub_cpu}" = "i386" -o "${grub_cpu}" = "arm64" ];then
		if [ "${grub_platform}" = "efi" ]; then                                     
			if [ "${grub_cpu}" != "arm64" ]; then                                   
				set linux=linuxefi                                                  
				set initrd=initrdefi                                                
			fi                                                                      
		fi                                                                          
	fi                                                                              
	if [ "${grub_platform}" = "efi" ]; then                                         
		echo "Please press 't' to show the boot menu on this console"               
	fi                                                                              
	set font=($root)/boot/${grub_cpu}/loader/grub2/fonts/unicode.pf2                
	if [ -f ${font} ];then                                                          
		loadfont ${font}                                                            
	fi                                                                              
	menuentry "%s" --class os --unrestricted {                                     
		echo Loading kernel...                                                      
		$linux ($root)/boot/kernel cdroot root=live:CDLABEL=%s rd.live.dir=/ rd.live.squashimg=rootfs.squashfs rd.live.overlay.overlayfs console=tty1 console=ttyS0 rd.cos.disable
		echo Loading initrd...                                                      
		$initrd ($root)/boot/initrd                                                 
	}                                                                               
																					
	if [ "${grub_platform}" = "efi" ]; then                                         
		hiddenentry "Text mode" --hotkey "t" {                                      
			set textmode=true                                                       
			terminal_output console                                                 
		}                                                                           
	fi`
	GrubBiosTarget  = "i386-pc"
	GrubI386BinDir  = "/usr/share/grub2/i386-pc"
	GrubBiosImg     = GrubI386BinDir + "/core.img"
	GrubBiosCDBoot  = GrubI386BinDir + "/cdboot.img"
	GrubEltoritoImg = GrubI386BinDir + "/eltorito.img"
	//TODO this list could be optimized
	GrubModules = "ext2 iso9660 linux echo configfile search_label search_fs_file search search_fs_uuid " +
		"ls normal gzio png fat gettext font minicmd gfxterm gfxmenu all_video xfs btrfs lvm luks " +
		"gcry_rijndael gcry_sha256 gcry_sha512 crypto cryptodisk test true loadenv part_gpt " +
		"part_msdos biosdisk vga vbe chain boot"

	IsoHybridMBR   = "/boot/x86_64/loader/boot_hybrid.img"
	IsoBootCatalog = "/boot/x86_64/boot.catalog"
	IsoBootFile    = "/boot/x86_64/loader/eltorito.img"

	// These paths are arbitrary but coupled to grub.cfg
	IsoKernelPath = "/boot/kernel"
	IsoInitrdPath = "/boot/initrd"

	// Default directory and file fileModes
	DirPerm        = os.ModeDir | os.ModePerm
	FilePerm       = 0666
	NoWriteDirPerm = 0555 | os.ModeDir
	TempDirPerm    = os.ModePerm | os.ModeSticky | os.ModeDir

	ArchAmd64 = "amd64"
	Archx86   = "x86_64"
	ArchArm64 = "arm64"
)

// GetDefaultSquashfsOptions returns the default options to use when creating a squashfs
func GetDefaultSquashfsOptions() []string {
	return []string{"-b", "1024k"}
}

func GetXorrisoBooloaderArgs(root string) []string {
	args := []string{
		"-boot_image", "grub", fmt.Sprintf("bin_path=%s", IsoBootFile),
		"-boot_image", "grub", fmt.Sprintf("grub2_mbr=%s/%s", root, IsoHybridMBR),
		"-boot_image", "grub", "grub2_boot_info=on",
		"-boot_image", "any", "partition_offset=16",
		"-boot_image", "any", fmt.Sprintf("cat_path=%s", IsoBootCatalog),
		"-boot_image", "any", "cat_hidden=on",
		"-boot_image", "any", "boot_info_table=on",
		"-boot_image", "any", "platform_id=0x00",
		"-boot_image", "any", "emul_type=no_emulation",
		"-boot_image", "any", "load_size=2048",
		"-append_partition", "2", "0xef", filepath.Join(root, IsoEFIPath),
		"-boot_image", "any", "next",
		"-boot_image", "any", "efi_path=--interval:appended_partition_2:all::",
		"-boot_image", "any", "platform_id=0xef",
		"-boot_image", "any", "emul_type=no_emulation",
	}
	return args
}
