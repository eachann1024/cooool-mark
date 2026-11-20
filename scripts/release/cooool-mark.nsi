Unicode true
!include "MUI2.nsh"

!ifndef VERSION
  !error "VERSION is required"
!endif
!ifndef OUTFILE
  !error "OUTFILE is required"
!endif
!ifndef BINARY
  !error "BINARY is required"
!endif

Name "Cooool Mark"
OutFile "${OUTFILE}"
InstallDir "$PROGRAMFILES64\Cooool Mark"
InstallDirRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\CoooolMark" "InstallLocation"
RequestExecutionLevel admin
VIProductVersion "${VERSION}.0"
VIAddVersionKey "ProductName" "Cooool Mark"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "ProductVersion" "${VERSION}"
VIAddVersionKey "FileDescription" "Cooool Mark installer"
VIAddVersionKey "LegalCopyright" "Cooool Mark"

!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "SimpChinese"

Section "Install"
  SetOutPath $INSTDIR
  File /oname=cooool-mark.exe "${BINARY}"
  CreateShortcut "$SMPROGRAMS\Cooool Mark.lnk" "$INSTDIR\cooool-mark.exe"
  WriteUninstaller "$INSTDIR\uninstall.exe"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\CoooolMark" "DisplayName" "Cooool Mark"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\CoooolMark" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\CoooolMark" "Publisher" "Cooool Mark"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\CoooolMark" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\CoooolMark" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\CoooolMark" "NoModify" 1
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\CoooolMark" "NoRepair" 1
  ; Remove any prior per-user (portable) protocol registration so HKLM takes effect cleanly.
  DeleteRegKey HKCU "Software\Classes\coooolmark"
  WriteRegStr HKLM "Software\Classes\coooolmark" "" "URL:Cooool Mark Protocol"
  WriteRegStr HKLM "Software\Classes\coooolmark" "URL Protocol" ""
  WriteRegStr HKLM "Software\Classes\coooolmark\DefaultIcon" "" "$\"$INSTDIR\cooool-mark.exe$\",0"
  WriteRegStr HKLM "Software\Classes\coooolmark\shell\open\command" "" "$\"$INSTDIR\cooool-mark.exe$\" $\"%1$\""
  ; Non-intrusive "Open with" ProgIDs for bookmark HTML and JSON files (preserves existing defaults).
  WriteRegStr HKLM "Software\Classes\app.cooool.mark.html" "" "Cooool Mark Bookmarks File"
  WriteRegStr HKLM "Software\Classes\app.cooool.mark.html\DefaultIcon" "" "$\"$INSTDIR\cooool-mark.exe$\",0"
  WriteRegStr HKLM "Software\Classes\app.cooool.mark.html\shell\open\command" "" "$\"$INSTDIR\cooool-mark.exe$\" $\"%1$\""
  WriteRegStr HKLM "Software\Classes\app.cooool.mark.json" "" "Cooool Mark Backup File"
  WriteRegStr HKLM "Software\Classes\app.cooool.mark.json\DefaultIcon" "" "$\"$INSTDIR\cooool-mark.exe$\",0"
  WriteRegStr HKLM "Software\Classes\app.cooool.mark.json\shell\open\command" "" "$\"$INSTDIR\cooool-mark.exe$\" $\"%1$\""
  WriteRegStr HKLM "Software\Classes\.html\OpenWithProgids" "app.cooool.mark.html" ""
  WriteRegStr HKLM "Software\Classes\.htm\OpenWithProgids" "app.cooool.mark.html" ""
  WriteRegStr HKLM "Software\Classes\.json\OpenWithProgids" "app.cooool.mark.json" ""
  System::Call 'shell32.dll::SHChangeNotify(i, i, i, i) v (0x08000000, 0, 0, 0)'
SectionEnd

Section "Uninstall"
  Delete "$INSTDIR\cooool-mark.exe"
  Delete "$INSTDIR\uninstall.exe"
  Delete "$SMPROGRAMS\Cooool Mark.lnk"
  RMDir "$INSTDIR"
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\CoooolMark"
  DeleteRegKey HKLM "Software\Classes\coooolmark"
  DeleteRegKey HKCU "Software\Classes\coooolmark"
  DeleteRegKey HKLM "Software\Classes\app.cooool.mark.html"
  DeleteRegKey HKLM "Software\Classes\app.cooool.mark.json"
  DeleteRegValue HKLM "Software\Classes\.html\OpenWithProgids" "app.cooool.mark.html"
  DeleteRegValue HKLM "Software\Classes\.htm\OpenWithProgids" "app.cooool.mark.html"
  DeleteRegValue HKLM "Software\Classes\.json\OpenWithProgids" "app.cooool.mark.json"
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "app.cooool.mark"
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "Cooool Mark"
  System::Call 'shell32.dll::SHChangeNotify(i, i, i, i) v (0x08000000, 0, 0, 0)'
SectionEnd
