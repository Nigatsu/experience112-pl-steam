# eXperience112 — spolszczenie wersji Steam

Program przenosi polską wersję językową (głosy, napisy, dokumenty, maile,
filmy, ruch ust) z **oryginalnej polskiej płyty** eXperience112 (Techland)
do wersji gry kupionej na **Steam**. Program nie zawiera żadnych danych gry —
wszystko odczytuje z Twoich własnych kopii.

## Wymagania

* Wersja Steam gry „Experience 112” w języku **angielskim**
  (Właściwości → Język → English) z oryginalnym plikiem
  `resources\loc_enUS.arc` (w razie wątpliwości: „Sprawdź spójność plików gry”).
  * wersja niemiecka i francuska mają inne pliki bazowe — program poprosi
    o przełączenie na angielski,
  * wersja hiszpańska (angielska + luźne pliki z tekstami) jest obsługiwana:
    luźne pliki zostaną przeniesione do kopii zapasowej.
* Obraz `.iso` polskiej płyty położony obok programu (ten sam folder, folder
  nadrzędny lub podfolder) — nie trzeba go montować. Może być też
  włożona/zamontowana płyta albo zainstalowana polska wersja.
* ok. 400 MB wolnego miejsca na dysku z grą.
* Prawo zapisu do folderu gry (przy błędzie „odmowa dostępu” uruchom
  program jako administrator).

## Użycie

1. Połóż obraz `.iso` polskiej płyty obok programu (albo włóż płytę).
2. Uruchom `exp112-pl-patch.exe` i odpowiadaj na pytania
   (Enter = odpowiedź domyślna). Program sam znajdzie grę w bibliotece Steam
   oraz obraz `.iso` / płytę.
3. Uruchom grę ze Steam.

**CrossOver (Mac):** uruchom program w TEJ SAMEJ butelce, w której jest Steam
(„Run Command” → wskaż plik `.exe`). Obraz `.iso` trzymaj w tym samym
folderze co program; obraz zamontowany w macOS też jest widoczny w butelce
jako napęd.

## Co robi program

* wypakowuje `loc_plPL.arc` z obrazu `.iso` lub płyty (z kontrolą sumy MD5),
* konwertuje teksty z UTF-16 na Windows-1250 (silnik wersji Steam jest ANSI),
* dodaje polskie litery do czcionek gry,
* 15 plików, których brak w wersji polskiej, zostaje po angielsku,
* przenosi `loc_enUS.arc` i `experience112.ini` do folderu `PL_patch_backup`
  (oraz ewentualne luźne pliki innych języków i archiwa `loc_*.arc`),
* ustawia `regionStrings` / `regionVoices` = `plPL`,
* poprawka ucinania dolnych części liter (g, j, y):
  * w CrossOver/Wine: rejestr butelki `VideoPciVendorID = 0x10de` (sprawdzone),
  * w Windows z kartą inną niż NVIDIA: łatka 3 bajtów w `engine.dll`
    (kopia w `PL_patch_backup`; ta opcja nie była testowana na Windows).

## Przywrócenie oryginalnych plików

```
exp112-pl-patch.exe -restore
```

„Sprawdź spójność plików gry” lub zmiana języka w Steam przywraca pliki
oryginalne — wtedy po prostu uruchom program ponownie (przy języku
angielskim).

## Opcje wiersza poleceń

```
  -game   <folder>   folder gry Steam (…\steamapps\common\Experience112)
  -source <ścieżka>  obraz .iso, płyta (folder z data1.hdr)
                     lub zainstalowana wersja PL
  -restore           przywróć oryginalne pliki gry
  -nogfx             bez poprawki ucinania liter
  -yes               bez pytań
```
