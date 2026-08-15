# Image Drop — Benutzerhandbuch

Version 0.1.3

Image Drop ist ein kleiner, schneller Bildbetrachter für macOS, Windows und
Linux. Es gibt keine Werkzeugleiste und keinen eingebauten Datei-Browser: Sie
ziehen Bilder auf das Fenster und sehen sie sich an. Das ist die ganze Idee —
wenn Sie aber nicht per Drag & Drop arbeiten möchten, öffnet ein Klick auf das
Fenster oder `Cmd`/`Strg+O` stattdessen den Dateiauswahldialog Ihres Systems
(siehe unten).

---

## 1. Erste Schritte

1. Starten Sie **Image Drop**. Ein kleines, leeres Fenster (etwa 520 × 340)
   erscheint mit einem abgerundeten Rahmen und dem Text **„Bilder hier
   ablegen“**.
2. Ziehen Sie ein oder mehrere Bilddateien aus Ihrem Dateimanager (Finder,
   Explorer, Nautilus, …) auf das Fenster und lassen Sie los.
3. Das erste Bild wird angezeigt, und das Fenster passt seine Größe
   automatisch daran an.

Sie können die Dateien an beliebiger Stelle auf dem Fenster ablegen — der
umrandete Bereich ist nur ein visueller Hinweis, kein Ziel, das Sie genau
treffen müssen.

**Keine Maus für Drag & Drop zur Hand, oder Sie bevorzugen einen
Dateiauswahldialog?** Klicken Sie irgendwo in den Ablegebereich, oder drücken
Sie `Cmd`/`Strg+O` (`Cmd`/`Strg+Shift+O` macht dasselbe — es ist eine zweite
Tastenkombination für denselben Dialog, kein eigener), um den Datei-Browser
Ihres Systems zu öffnen. Unter macOS und Linux können Sie damit eine beliebige
Mischung aus Dateien und Ordnern auf einmal auswählen, genau wie per Drag &
Drop; Ordner werden auf dieselbe Weise eingelesen (siehe „Ordner einlesen“
unten). **Unter Windows bietet der Dialog nur Dateien an** — der
Windows-eigene Dateidialog hat keinen Modus, der Ordner- und Mehrfachauswahl
kombiniert —, Ordner fügen Sie dort per Drag & Drop hinzu. Das funktioniert
jederzeit, nicht nur auf dem leeren Ablegebildschirm — weitere Bilder zu
öffnen, während bereits eines angezeigt wird, ersetzt die aktuelle Auswahl
oder ergänzt sie, wenn der Zusammenführen-Modus (`M`) aktiv ist, genau wie
beim Ablegen neuer Dateien.

---

## 2. Dieses Handbuch öffnen

Sie lesen es gerade, haben es also vermutlich schon gefunden, aber der
Vollständigkeit halber:

- Drücken Sie jederzeit **`F1`**, oder
- wählen Sie **Hilfe -> Handbuch** aus dem Menü.

Das Handbuch öffnet sich in einem eigenen, scrollbaren Fenster. Drücken Sie
**`Esc`** oder schließen Sie das Fenster, um es zu verlassen; das Bild, das
Sie betrachtet haben, bleibt unverändert. Ein erneuter Druck auf `F1` holt das
bereits geöffnete Handbuchfenster nach vorne, statt eine zweite Kopie zu
öffnen.

---

## 3. Unterstützte Dateiformate

- **JPEG** — `.jpg`, `.jpeg`, `.jpe`, `.jfif` (EXIF-Rotation wird angewendet)
- **PNG** — `.png` (Transparenz wird unterstützt)
- **GIF** — `.gif` (animierte GIFs werden abgespielt)
- **WebP** — `.webp` (statische Bilder)
- **BMP** — `.bmp`
- **TIFF** — `.tif`, `.tiff`
- **ICO** — `.ico` (Windows-Symbol; das größte enthaltene Bild wird angezeigt)
- **XPM** — `.xpm` (X Pixmap)
- **HEIC/HEIF** — `.heic`, `.heif` (iPhone-Fotos; EXIF-Rotation wird
  angewendet)
- **AVIF** — `.avif` (eingebaute Rotation/Spiegelung wird angewendet)

Eine Datei wird auch akzeptiert, wenn Ihr System sie als `image/jpeg`,
`image/png`, `image/gif`, `image/webp`, `image/bmp`, `image/tiff`,
`image/x-icon`, `image/vnd.microsoft.icon`, `image/x-xpixmap`, `image/heic`,
`image/heif` oder `image/avif` meldet, auch wenn die Dateiendung fehlt oder
ungewöhnlich ist.

Alles andere — PDFs, RAW-Kameradateien, SVG, Videos — wird **nicht**
unterstützt.

---

## 4. Bilder betrachten

### Automatische Fenstergröße

Jedes Mal, wenn ein Bild angezeigt wird, passt sich die Fenstergröße daran
an:

- **Große Bilder** werden so verkleinert, dass sie in **1500 × 950** Pixel
  passen, das Seitenverhältnis bleibt erhalten.
- **Kleine Bilder** verkleinern das Fenster nie unter die Startgröße
  (520 × 340). Ein winziges Vorschaubild wird zentriert, mit leerem Platz
  drumherum, statt ein Fenster zu erzeugen, das zu klein zum Anfassen wäre.
- Sie können das Fenster jederzeit selbst durch Ziehen am Rand vergrößern
  oder verkleinern. Das Bild wird passend skaliert und nie beschnitten oder
  verzerrt.

### Die Fenstertitelzeile

Die Titelzeile zeigt, was Sie gerade betrachten, zum Beispiel:

`sunset.jpg — 4032 x 3024  (2/7)`

- **Dateiname** des aktuellen Bildes
- **Pixelabmessungen** des Bildes (nach etwaiger Rotationskorrektur)
- **`(animated)`**, wenn es sich um ein animiertes GIF handelt
- **`(2/7)`** — Position in der aktuellen Auswahl, wird nur angezeigt, wenn
  Sie mehr als ein Bild abgelegt haben
- **`[Zusammenführen]`** ganz vorne, nur solange der Zusammenführen-Modus
  (`M`) aktiv ist — siehe „Mehrere Bilder durchblättern“ unten

### Fotorotation

Fotos, die im Hoch- oder Querformat mit einem Handy oder einer Kamera
aufgenommen wurden, tragen ein EXIF-Orientierungs-Tag. Image Drop liest
dieses Tag und dreht oder spiegelt das Foto automatisch, sodass
Hochformataufnahmen aufrecht statt seitlich liegend erscheinen. Alle acht
EXIF-Orientierungen werden unterstützt, und die Abmessungen in der Titelzeile
spiegeln das korrigierte Bild wider.

Wenn ein Foto trotzdem nicht so ausgerichtet ist, wie Sie es möchten —
EXIF-korrigiert, aber aus einem anderen Grund seitlich, oder Sie möchten es
einfach gedreht betrachten —, drücken Sie **`R`**, um es um weitere 90° im
Uhrzeigersinn zu drehen, oder **`Shift+R`** für gegen den Uhrzeigersinn. Das
ist rein eine Ansichtsoption: Sie wird nie in die Datei zurückgeschrieben,
wird mit **`0`** (zusammen mit dem Zoom) wieder auf aufrecht zurückgesetzt
und beginnt — wie beim Wechsel zum nächsten Bild — beim nächsten
betrachteten Foto wieder aufrecht. Beim Drehen wechselt das Fenster zwischen
Quer- und Hochformat, um sich der neuen Ausrichtung des Bildes anzupassen,
genau wie beim Laden eines anderen Bildes.

### Animierte GIFs

Animierte GIFs beginnen zu spielen, sobald sie angezeigt werden:

- Die Einzelbilder laufen mit der in der Datei gespeicherten Geschwindigkeit.
- Einzelbilder, die nur einen Teil des Bildes aktualisieren, werden korrekt
  zusammengesetzt, sodass Sie keine Bildreste oder flackernde leere Stellen
  sehen.
- Einzelbilder ohne Verzögerung (oder mit einer Verzögerung von null) werden
  0,1 s lang angezeigt, damit die Wiedergabe flüssig bleibt.
- Die Animation läuft in einer Schleife, bis Sie weiterblättern oder neue
  Dateien ablegen — dann stoppt sie von selbst.

---

## 5. Zoom und Verschieben

Standardmäßig wird ein Bild immer **fensterfüllend eingepasst**, wie oben
beschrieben. Vier Tasten wechseln zu einer manuellen Zoomstufe:

- **`+`** — vergrößern
- **`-`** — verkleinern
- **`1`** — direkt auf **100 %** springen (ein Bildpixel pro Bildschirmpixel)
- **`0`** — zurück zur Fenstereinpassung

Der erste Druck auf `+` oder `-` zoomt von der aktuell eingepassten Ansicht
aus, statt zuerst auf 100 % zu springen, damit sich das Zoomen fließend
anfühlt. Wiederholtes Drücken skaliert weiter hoch oder herunter, begrenzt
auf 5 % bis 1600 %.

**Scrollen** mit Mausrad oder Trackpad über dem Bild zoomt ebenfalls, und
anders als die Tastenkürzel zoomt es um den Punkt unter dem Mauszeiger statt
um die Bildmitte, sodass das, worauf Sie zeigen, beim Zoomen an derselben
Stelle auf dem Bildschirm bleibt.

Sobald das Bild so weit hineingezoomt ist, dass es nicht mehr ins Fenster
passt, wird der Mauszeiger zu einer Hand, um anzuzeigen, dass es verschoben
werden kann; **klicken und ziehen** Sie, um es zu verschieben — die Bewegung
ist begrenzt, sodass Sie das Bild nicht so weit ziehen können, dass leerer
Platz dahinter entsteht. Solange das Bild eingepasst ist oder eine Zoomstufe
hat, bei der es noch ins Fenster passt, bewirkt Verschieben nichts, und der
Mauszeiger bleibt der normale Pfeil.

Wenn Sie **Shift** beim Scrollen gedrückt halten, wird verschoben statt
gezoomt, in welche Richtung auch immer Sie scrollen — praktisch für eine
Zweifinger-Wischgeste auf dem Trackpad, sobald Sie hineingezoomt haben, ohne
klicken und ziehen zu müssen.

Zoom und Verschiebung gelten pro Bild: Der Wechsel zu einem anderen Bild (mit
den Pfeiltasten, `Home`/`End` oder einem neuen Ablegen) setzt dieses Bild
immer wieder auf Fenstereinpassung zurück. Die Ansichtsrotation (siehe
„Fotorotation“ oben) wird auf dieselbe Weise zurückgesetzt, und das Drehen
setzt auch den Zoom auf die Einpassung zurück — eine vor der Drehung gewählte
manuelle Zoomstufe ergibt selten noch Sinn, sobald die Hoch-/Querformatachsen
des Bildes vertauscht wurden.

---

## 6. Info-Overlay

Drücken Sie **`I`**, um in der oberen linken Ecke des Fensters eine kleine
Karte mit allen Informationen zum aktuellen Bild auf einen Blick
einzublenden:

- den **Dateinamen** und seine Position in der Auswahl (z. B. `3 / 47`), wenn
  Sie mehr als ein Bild abgelegt haben
- seine **Pixelabmessungen** (z. B. `1920 x 1080`)
- seine **Dateigröße** auf der Festplatte
- die aktuelle **Zoomstufe** in Prozent

Es wird live aktualisiert, während Sie navigieren oder den Zoom ändern, und
bleibt — anders als eine Toast-Meldung — so lange sichtbar, bis Sie `I`
erneut drücken, um es auszublenden. Es ist eine dauerhafte Einstellung wie
die Sortierreihenfolge oder der Zusammenführen-Modus: Einmal eingeschaltet,
bleibt es über Navigation und weitere Ablagen hinweg an und erscheint wieder,
sobald das nächste Bild geladen wird, selbst wenn Sie zwischendurch kurz zum
leeren Ablegebildschirm zurückkehren.

Unter dieser Übersicht öffnet ein Link **„EXIF-Daten anzeigen“** ein
separates Fenster mit den Exif-Metadaten des aktuellen Bildes —
Kamerahersteller und -modell, Objektiv, Belichtungszeit, Blende, ISO,
Brennweite und Aufnahmedatum, eine Zeile pro Tag, das tatsächlich in der
Datei vorhanden ist. `E` öffnet dasselbe Fenster direkt, ohne dass das
Info-Overlay vorher geöffnet sein muss. GPS-Standortdaten werden absichtlich
nie gelesen oder angezeigt. Das Fenster aktualisiert sich, wenn Sie bei
geöffnetem Fenster zu einem anderen Bild wechseln, und — wie beim Handbuch-
und Info-Fenster — schließt `Esc` nur dieses Fenster, und ein erneuter Druck
auf `E`, während es bereits offen ist, holt es nach vorne, statt eine zweite
Kopie zu öffnen. Dateien ohne Exif-Daten (die meisten PNGs, GIFs und WebPs
sowie jedes JPEG ohne von einer Kamera geschriebenes Exif-Segment) zeigen
stattdessen die Meldung „keine Metadaten gefunden“.

---

## 7. Mehrere Bilder durchblättern

Legen Sie mehrere Dateien auf einmal ab und blättern Sie mit der Tastatur
durch:

- **`→`** oder **`↓`** — nächstes Bild
- **`←`** oder **`↑`** — vorheriges Bild
- **`Home`** — zum ersten Bild springen
- **`End`** — zum letzten Bild springen

Die Navigation **läuft im Kreis**: `→` beim letzten Bild springt zurück zum
ersten, `←` beim ersten Bild zum letzten.

Hinweise:

- Die Pfeiltasten haben nur eine Wirkung, wenn Sie **zwei oder mehr** Bilder
  abgelegt haben.
- Standardmäßig ist die Auswahl **natürlich sortiert** nach Dateiname, sodass
  `IMG_2.jpg` vor `IMG_10.jpg` kommt, obwohl eine reine Textsortierung sie in
  der anderen Reihenfolge anordnen würde. Drücken Sie **`S`**, um durch vier
  weitere Sortierungen und zurück zum Namen zu wechseln:
  - **Aufnahmedatum** — das Exif-Datum/-Zeit des Fotos (derselbe Wert, den
    das Exif-Fenster als „Aufnahmedatum“ zeigt); eine Datei ohne
    Exif-Aufnahmedatum — ein Screenshot, die meisten PNGs/GIFs/WebPs, oder
    ein JPEG, das eine Kamera nie getaggt hat — greift stattdessen auf den
    Änderungszeitpunkt im Dateisystem zurück, statt sich ganz am Anfang der
    Liste zu häufen.
  - **Geändert** — Änderungszeitpunkt im Dateisystem.
  - **Größe** — Dateigröße, kleinste zuerst.
  - **Unsortiert** — die Reihenfolge, in der Ihr Dateimanager die Dateien
    roh übergeben hat („dumme Sortierung“ — gar keine Sortierung).

  Die Titelzeile zeigt an, welcher Modus aktiv ist (`[Sortierung: Datum]`,
  `[Sortierung: Geändert]`, `[Sortierung: Größe]`, `[unsortiert]`) — für die
  Standard-Namenssortierung wird nichts angezeigt. Das Bild, das Sie gerade
  betrachten, bleibt bei jedem Wechsel auf dem Bildschirm. Die Sortierung
  entfernt nie doppelte Dateien, und die Einstellung bleibt bis zum nächsten
  Ablegen und über Neustarts hinweg erhalten, bis Sie sie wieder ändern.

  Aufnahmedatum, Änderungszeitpunkt und Größe müssen jeweils jede Datei
  einmal lesen, um danach zu sortieren — ein Stat-Aufruf für
  Änderungszeitpunkt/Größe, ein Rohdatei-Lesevorgang für das Aufnahmedatum —,
  was bei einem sehr großen rekursiven Ablegen spürbar pausieren kann, ohne
  Fortschrittsanzeige oder Möglichkeit, es nach dem Start abzubrechen.
- Neue Dateien abzulegen **ersetzt** die aktuelle Auswahl und beginnt wieder
  beim ersten gerade abgelegten Bild, sofern der **Zusammenführen-Modus**
  nicht aktiv ist. Drücken Sie **`M`**, um den Zusammenführen-Modus ein- oder
  auszuschalten; solange er aktiv ist, beginnt die Titelzeile mit
  **`[Zusammenführen]`**, sodass Sie immer erkennen, in welchem Modus Sie
  sich befinden. Mit aktivem Zusammenführen-Modus **ergänzt** ein neues
  Ablegen seine Dateien zur aktuellen Auswahl, statt sie zu ersetzen — die
  Anzeige springt zur ersten gerade hinzugefügten Datei, die Sortierung gilt
  weiterhin, und nichts wird dedupliziert, sodass das zweimalige Ablegen
  derselben Datei sie auch zweimal hinzufügt. Enthält ein Ablegen im
  Zusammenführen-Modus nichts Unterstütztes, bleibt die bestehende Auswahl
  unverändert, und Sie erhalten nur eine Fehler-Toast-Meldung, keine
  Löschung. Der Zusammenführen-Modus ist eine dauerhafte Einstellung wie die
  Sortierreihenfolge — er bleibt über mehrere Ablagen hinweg ein- (oder
  aus-)geschaltet, bis Sie `M` erneut drücken, sodass Sie beim Ziehen nichts
  gedrückt halten müssen.

---

## 8. Rasteransicht

Drücken Sie **`G`**, um zu einem fensterfüllenden Raster von
Miniaturansichten der aktuellen Auswahl zu wechseln — praktisch, um bei
einem großen Ablegen ein bestimmtes Bild visuell zu finden, statt sich
einzeln durchzublättern.

- Klicken Sie auf eine Miniaturansicht, um direkt zu ihr zu springen und zur
  normalen Ansicht zurückzukehren, oder nutzen Sie die Tastatur: Die
  Pfeiltasten bewegen einen hervorgehobenen Rahmen durch das Raster
  (beginnend bei dem Bild, das beim Öffnen gerade angezeigt wurde), und
  **`Return`** öffnet die gerade hervorgehobene Miniaturansicht.
- Drücken Sie **`G`** erneut, oder **`Esc`**, um das Raster ohne Auswahl zu
  verlassen.
- Solange das Raster geöffnet ist, wird jede andere Taste ignoriert — Zoom,
  `S`/`M`/`P`/`I`, `Shift+Delete` bewirken nichts, bis Sie entweder eine
  Miniaturansicht auswählen (Klick oder `Return`) oder mit `G`/`Esc`
  zurückgehen.
- Miniaturansichten werden im Hintergrund erzeugt, sobald sie ins Blickfeld
  scrollen, jeweils einige auf einmal, sodass das Öffnen des Rasters bei
  einem Ordner mit Tausenden von Bildern das Fenster nicht blockiert,
  während alle im Voraus dekodiert werden.
- Das Raster benötigt mindestens ein geladenes Bild und lässt sich nicht mit
  dem Diaschau-Modus kombinieren — das Öffnen des einen schließt das andere.

---

## 9. Diaschau-Modus

Drücken Sie **`P`**, um die aktuelle Bildauswahl in eine Vollbild-Diaschau zu
verwandeln — praktisch, um Image Drop einfach dastehen und durch einen
Ordner voller Fotos wie einen digitalen Bilderrahmen laufen zu lassen.

- Das Fenster wechselt in den **Vollbildmodus**. Das Bild wird
  bildschirmfüllend skaliert, das Seitenverhältnis bleibt erhalten — nie
  gestreckt oder beschnitten, dasselbe Einpassverhalten wie im normalen
  Fenster.
- Alle **10 Sekunden** (standardmäßig) wechselt die Ansicht **automatisch**
  zum nächsten Bild, am Ende beginnt es wieder von vorne, genau wie bei
  manueller Navigation. Jeder Wechsel wird **überblendet** — das
  ausscheidende Bild verblasst, das neue blendet ein — statt des sofortigen
  Wechsels beim normalen Durchblättern. Auch die manuelle Navigation
  (`←`/`→`/`Home`/`End`) wird während des Diaschau-Modus auf dieselbe Weise
  überblendet.
- **`↑`** erhöht das Intervall um eine Sekunde, **`↓`** verringert es (bis
  zu einer Untergrenze von einer Sekunde). Solange der Diaschau-Modus aktiv
  ist, steuern `↑`/`↓` den Timer statt zu navigieren — nutzen Sie
  **`←`**/**`→`** (oder `Home`/`End`) zum manuellen Navigieren, das
  weiterhin wie gewohnt funktioniert und den Countdown ab dem neuen Bild neu
  startet.
- **`Shift+P`** schaltet **Zufällige Wiedergabe** ein oder aus: Ist sie
  aktiv, wählt der automatische Wechsel jedes Mal ein zufälliges anderes
  Bild statt des nächsten in der Reihenfolge (nie das gerade angezeigte),
  und die Titelzeile beginnt mit **`[Zufällig]`**. Die manuelle Navigation
  mit `←`/`→`/`Home`/`End` bleibt davon unberührt — sie durchläuft die
  Auswahl immer in Reihenfolge. Die Zufällige Wiedergabe verhält sich wie
  eine dauerhafte Einstellung, genau wie der Zusammenführen-Modus:
  `Shift+P` funktioniert schon, bevor Sie den Diaschau-Modus überhaupt
  einschalten, und auch außerhalb davon.
- **Animierte GIFs werden immer zu Ende abgespielt.** Wenn ein
  GIF-Durchlauf länger dauert als das aktuelle Intervall, wartet der
  Diaschau-Modus, bis er mindestens einmal komplett durchgelaufen ist, statt
  ihn mittendrin abzubrechen.
- Ihr gewähltes Intervall und die Einstellung der Zufälligen Wiedergabe
  werden beim nächsten Einschalten des Diaschau-Modus gemerkt — und bleiben
  auch beim nächsten Start von Image Drop erhalten.
- Drücken Sie **`P`** erneut, oder **`Esc`**, um den Diaschau-Modus zu
  verlassen und zum normalen Fenster zurückzukehren. `Esc` verlässt hier nur
  den Diaschau-Modus — es leert nicht auch die geladenen Bilder; drücken Sie
  es danach erneut, um das zu tun.
- Der Diaschau-Modus benötigt mindestens ein geladenes Bild — `P` auf dem
  leeren Ablegebildschirm bewirkt nichts.

---

## 10. Eine Datei löschen

Drücken Sie **`Shift+Delete`**, um die gerade angezeigte Datei dauerhaft von
der Festplatte zu löschen — nicht in den Papierkorb des Betriebssystems, es
gibt also kein Rückgängig. Eine Bestätigungskarte erscheint mit zwei
Schaltflächen:

- **Abbrechen** — standardmäßig ausgewählt
- **Endgültig löschen** (in Rot)

Die gerade ausgewählte Schaltfläche ist umrandet, sodass immer sichtbar ist,
was `Return` auslösen wird, bevor Sie es drücken.

Sie können auf beide Arten antworten:

- **Mit der Maus**: Klicken Sie direkt auf eine der beiden Schaltflächen.
- **Mit der Tastatur**: Drücken Sie **`→`**, um die Auswahl auf „Endgültig
  löschen“ zu verschieben (**`←`** verschiebt sie zurück auf „Abbrechen“) —
  der Rahmen bewegt sich mit — und dann **`Return`**, um mit der jeweils
  ausgewählten Option fortzufahren. **`Esc`** bricht sofort ab, egal welche
  Option gerade ausgewählt ist.

Solange die Karte angezeigt wird, wird jede andere Taste ignoriert —
Navigation, Zoom, `S`/`M`/`P`/`I`/`G` bewirken nichts, bis Sie die Nachfrage
auf die eine oder andere Weise beantworten. `Shift+Delete` selbst ist an
anderer Stelle in der App weiterhin erreichbar, bewirkt aber nichts, solange
die Rasteransicht (`G`, siehe oben) angezeigt wird.

Das Löschen der aktuellen Datei entfernt sie aus der Auswahl und zeigt, was
jetzt an ihre Stelle tritt, wobei die Navigation im Kreis läuft, genau wie
sonst; war sie die letzte verbliebene Datei, gelangen Sie zurück zum leeren
Ablegebildschirm. Stellt sich heraus, dass die Datei bereits verschwunden
ist, oder kann sie aus einem anderen Grund nicht gelöscht werden (zum
Beispiel wegen Berechtigungen), erklärt eine Toast-Meldung, was schiefgegangen
ist, und die Datei bleibt in der Auswahl.

---

## 11. Tastenkürzel

- **`F1`** — dieses Handbuch öffnen
- **`Cmd`/`Strg+O`** / **`Cmd`/`Strg+Shift+O`** — den System-Dateidialog
  öffnen (dasselbe wie ein Klick in den Ablegebereich; beide
  Tastenkombinationen bewirken dasselbe; Dateien und Ordner unter
  macOS/Linux, nur Dateien unter Windows — siehe oben)
- **`→`** / **`↓`** — nächstes Bild
- **`←`** / **`↑`** — vorheriges Bild
- **`Home`** / **`End`** — erstes / letztes Bild
- **`S`** — Sortierreihenfolge durchschalten: Name -> Aufnahmedatum ->
  Änderungszeitpunkt -> Größe -> unsortiert -> zurück zu Name
- **`M`** — Zusammenführen-Modus ein-/ausschalten (neues Ablegen ergänzt die
  Auswahl, statt sie zu ersetzen); wird in der Titelzeile mit dem Präfix
  **`[Zusammenführen]`** angezeigt
- **`G`** — Rasteransicht ein-/ausschalten (siehe oben); Pfeiltasten bewegen
  die Hervorhebung, `Return` oder ein Klick öffnet sie, `G`/`Esc` bricht ab
- **`+`** / **`-`** — vergrößern / verkleinern
- **`1`** — auf 100 % zoomen; **`0`** — zurück zur Fenstereinpassung (und
  setzt die Rotation zurück, siehe unten)
- Scrollen (Mausrad oder Trackpad) — vergrößern/verkleinern, verankert am
  Mauszeiger
- **Shift** + Scrollen — verschieben statt zoomen
- Klicken und ziehen — ein hineingezoomtes Bild verschieben
- **`R`** / **`Shift+R`** — das angezeigte Bild um 90° im/gegen den
  Uhrzeigersinn drehen (nur Ansicht; wird bei `0` oder dem nächsten Bild
  zurückgesetzt)
- **`I`** — Info-Overlay ein-/ausschalten (Dateiname, Position, Abmessungen,
  Dateigröße, Zoomstufe)
- **`E`** — das EXIF-Datenfenster für das aktuelle Bild öffnen
  (Kamerahersteller/-modell, Objektiv, Belichtung, Blende, ISO, Brennweite,
  Aufnahmedatum); auch über den Link **„EXIF-Daten anzeigen“** im
  Info-Overlay erreichbar
- **`Cmd`/`Strg+C`** — das aktuelle Bild in die Systemzwischenablage
  kopieren, als Bilddaten, die Sie in eine andere App einfügen können (keine
  Datei)
- **`Cmd`/`Strg+Shift+C`** — den Dateipfad des aktuellen Bildes in die
  Zwischenablage kopieren
- **`Shift+Delete`** — die aktuelle Datei nach Bestätigung dauerhaft von der
  Festplatte löschen (siehe „Eine Datei löschen“ oben)
- **`P`** — Diaschau-Modus ein-/ausschalten (Vollbild-Diaschau mit
  Überblendung zwischen den Bildern, siehe oben)
- **`Shift+P`** — Zufällige Wiedergabe für den automatischen Wechsel im
  Diaschau-Modus ein-/ausschalten; wird in der Titelzeile mit dem Präfix
  **`[Zufällig]`** angezeigt
- **`↑`** / **`↓`** *(im Diaschau-Modus)* — das Auto-Weiterschalt-Intervall
  um eine Sekunde erhöhen/verringern
- **`Esc`** — die aktuellen Bilder leeren und zum anfänglichen
  Ablegebildschirm zurückkehren; beendet die App, wenn nichts geladen ist,
  das geleert werden könnte (im Handbuchfenster schließt es nur das
  Handbuch; im Diaschau-Modus verlässt es zuerst den Diaschau-Modus);
  solange noch ein Ordner-Scan läuft, bricht es stattdessen den Scan ab
  (siehe „Ordner einlesen“ unten)

**Zwischenablage unter Linux.** Das Kopieren des Bildes selbst (`Strg+C`)
ruft ein externes Werkzeug auf, da Linux keinen einzigen eingebauten Weg hat,
um Bilddaten in die Zwischenablage zu legen: `xclip`, oder als Fallback
`wl-copy` (aus dem Paket `wl-clipboard`) für eine Wayland-Sitzung ohne
XWayland. Die meisten Distributionen installieren standardmäßig keines von
beiden — installieren Sie eines mit Ihrem Paketmanager, z. B.
`sudo apt install xclip` oder `sudo apt install wl-clipboard` unter
Debian/Ubuntu. Ohne eines der beiden zeigt `Strg+C` eine Fehler-Toast-Meldung,
statt zu kopieren. Das Kopieren des Dateipfads (`Strg+Shift+C`) ist reiner
Text und funktioniert immer, ohne zusätzliches Werkzeug. macOS und Windows
benötigen in beiden Fällen nichts Zusätzliches.

---

## 12. Menü

- **Hilfe -> Handbuch** — öffnet dieses Handbuch, genau wie `F1`

---

## 13. Rückmeldung beim Laden

Das Dekodieren läuft im Hintergrund, sodass das Fenster auch bei sehr großen
Dateien reaktionsfähig bleibt.

- Ein animierter **Fortschrittsbalken** erscheint am oberen Rand des
  Fensters, während ein Bild geladen wird, und verschwindet, sobald es
  fertig ist. Er wird über das Bild gelegt und verschiebt nie etwas auf dem
  Bildschirm.
- Beim allerersten Ablegen ändert sich der Hinweistext zu **„Wird
  geladen…“**.
- Bei späteren Wechseln bleibt das vorherige Bild sichtbar, bis das neue
  bereit ist, sodass es keinen leeren Blitzer zwischen den Bildern gibt.
- Tastendrücke, während ein Bild noch lädt, werden ignoriert. Das
  Gedrückthalten einer Pfeiltaste staut daher keinen Rückstand an
  Dekodierungen für Bilder an, die Sie bereits übersprungen haben.
- Wird ein langsames Bild fertig dekodiert, nachdem Sie bereits weitergegangen
  sind, wird das Ergebnis verworfen — Sie sehen immer das zuletzt ausgewählte
  Bild.

**Ordner einlesen.** Enthält Ihr Ablegen Ordner, durchsucht Image Drop diese
zuerst (und jeden Unterordner), um unterstützte Bilder zu sammeln, bevor
irgendetwas angezeigt wird:

- Ein Spinner erscheint zusammen mit einem laufenden Zähler, z. B.
  **„Scannen… 42 Bilder“**, der aktualisiert wird, sobald weitere Bilder
  gefunden werden.
- Sobald der Scan abgeschlossen ist, verschwindet der Spinner, und das erste
  gefundene Bild wird angezeigt, genau wie bei einem normalen Ablegen.
- Ablegen ohne Ordner überspringt diesen Schritt vollständig und lädt
  sofort.
- Drücken Sie jederzeit **`Esc`**, während der Spinner angezeigt wird, um
  den Scan abzubrechen. War dies das allererste Ablegen, gelangen Sie zurück
  zum anfänglichen Ablegebildschirm, genau als wäre nichts abgelegt worden;
  haben Sie in eine bereits geladene Auswahl zusammengeführt, bleiben die
  Bilder, die Sie vor Beginn des Scans hatten, unangetastet.

---

## 14. Meldungen und Fehlerbehandlung

Image Drop zeigt in folgenden Fällen ein Dialogfenster an. Schließen Sie es
mit der Schaltfläche **OK**.

- **Eine nicht unterstützte Datei abgelegt** — *„…“ ist keine unterstützte
  Bilddatei*
- **Mehrere Dateien, keine davon unterstützt** — *Keine der N abgelegten
  Dateien ist ein unterstütztes Bildformat*
- **Datei kann nicht gelesen oder dekodiert werden** — *„…“ konnte nicht
  gelesen werden*
- **Datei dekodiert zu einem Bild der Größe null** — *Ungültige
  Bildabmessungen für „…“*

**Gemischte Ablagen werden stillschweigend behandelt.** Wenn Sie einen
Stapel Fotos zusammen mit ein paar Textdateien ablegen, werden die
unterstützten Bilder angezeigt, und der Rest wird ohne Dialog übersprungen.
Nur ein Ablegen, das **kein** verwendbares Bild enthält, erzeugt einen
Fehler.

**Eine Datei, die sich nicht dekodieren lässt, wird aus der Auswahl
entfernt.** Stellt sich erst beim Navigieren zu einer Datei heraus, dass sie
unlesbar oder beschädigt ist (eine Prüfung anhand der Dateiendung beim
Ablegen kann nicht alles abfangen), wird sie aus der Auswahl entfernt, und
das nächste Bild wird automatisch angezeigt, wobei die Navigation im Kreis
läuft, falls es die letzte Datei war — Sie stehen nie vor einer defekten
Datei, bei der Titelzeile und Positionszähler nicht mehr zu dem passen, was
tatsächlich angezeigt wird. Sie erhalten lediglich eine Toast-Meldung mit dem
Namen der übersprungenen Datei. Erweist sich jede Datei in der Auswahl als
defekt, gelangen Sie zurück zum anfänglichen Ablegebildschirm.

**Abgelegte Ordner werden aufgelöst.** Das Ablegen eines Ordners durchsucht
ihn und jeden Unterordner nach unterstützten Bildern; Sie können eine
beliebige Mischung aus einzelnen Bilddateien und Ordnern auf einmal ablegen.
Siehe „Ordner einlesen“ oben für das, was Sie sehen, während ein Ordner
durchsucht wird.

---

## 15. Sprache

Der Oberflächentext (der Ablege-Hinweis, „Wird geladen…“, das Menü) kann
übersetzt werden. Image Drop wird mit Englisch ausgeliefert und folgt Ihrer
Systemsprache, sofern eine passende Übersetzung verfügbar ist; andernfalls
greift es auf Englisch zurück. Dieses Handbuch folgt derselben Regel: Bei
deutscher Systemsprache öffnet sich diese deutsche Fassung, sonst die
englische.

---

## 16. Beenden

Drücken Sie `Esc` im Bildfenster, wenn nichts geladen ist, oder schließen
Sie es auf die für Ihre Plattform übliche Weise (die rote Schaltfläche unter
macOS, das ✕ unter Windows und Linux). Sind Bilder geladen, leert `Esc` sie
zunächst und kehrt zum anfänglichen Ablegebildschirm zurück — drücken Sie es
erneut (jetzt, da die Auswahl geleert ist), um die App zu beenden. Der
Zusammenführen-Modus, die Sortierreihenfolge, das Diaschau-Intervall samt
Zufälliger Wiedergabe und die Fenstergröße bleiben bis zum nächsten Start
erhalten (siehe die jeweiligen Abschnitte oben); alles andere nicht — Zoom,
Rotation und das zuletzt betrachtete Bild werden zurückgesetzt.

---

## 17. Aktuelle Einschränkungen

Dinge, die Image Drop absichtlich (noch) nicht tut:

- Keine Navigation per Mausrad oder Trackpad-Scroll; das Durchblättern der
  Bilder funktioniert nur über die Tastatur (Pfeiltasten, `Home`/`End`,
  siehe oben)
- Keine echte Pinch-to-Zoom-Trackpad-Geste; Shift+Scrollen ist der
  nächstliegende Ersatz für Zweifinger-Verschieben (siehe „Zoom und
  Verschieben“ oben)
- Keine Zoomsteuerung innerhalb des Diaschau-Modus selbst, und keine
  bildspezifische Zeitsteuerung — jedes Bild erhält dasselbe Intervall
  (animierte GIFs ausgenommen)
- Kein Bearbeiten, Konvertieren oder Speichern von Dateien, und die Rotation
  (`R`/`Shift+R`, siehe „Fotorotation“ oben) ist nur eine Ansichtsoption,
  wird nie auf die Festplatte geschrieben (Löschen wird unterstützt — siehe
  „Eine Datei löschen“ oben —, alles andere nicht)
- Keine Unterstützung für RAW, SVG oder PDF
- Keine Wiedergabesteuerung (Pause, Einzelschritt, Neustart) für animierte
  GIFs
- Keine EXIF-GPS-/Standortanzeige — absichtlich aus dem EXIF-Datenfenster
  ausgelassen (siehe „Info-Overlay“ oben) zum Schutz der Privatsphäre

---

## 18. Kurzübersicht

- **Laden** — Bilddateien auf das Fenster ziehen (ersetzt die aktuelle
  Auswahl)
- **Öffnen** — auf den Ablegebereich klicken, oder `Cmd`/`Strg+O` drücken
  (oder `Cmd`/`Strg+Shift+O`, dasselbe), für den System-Dateidialog (Dateien
  und Ordner unter macOS/Linux, nur Dateien unter Windows)
- **Zusammenführen-Modus** — `M` schaltet ihn ein/aus; solange aktiv,
  ergänzen Ablagen die Auswahl, statt sie zu ersetzen, und die Titelzeile
  zeigt `[Zusammenführen]`
- **Nächstes / Vorheriges** — `→` `↓` / `←` `↑` (läuft im Kreis)
- **Erstes / Letztes** — `Home` / `End`
- **Sortierreihenfolge** — `S` schaltet durch Name → Aufnahmedatum →
  Änderungszeitpunkt → Größe → unsortiert → zurück zu Name
- **Rasteransicht** — `G` schaltet ein fensterfüllendes Miniaturraster
  ein/aus; Pfeiltasten bewegen die Hervorhebung, `Return` oder ein Klick
  öffnet, `G`/`Esc` bricht ohne Auswahl ab
- **Zoom** — `+`/`-` vergrößern/verkleinern, `1` für 100 %, `0` für
  Fenstereinpassung, oder Scrollen zum Zoomen am Mauszeiger; ziehen, oder
  Shift+Scrollen, zum Verschieben nach dem Hineinzoomen
- **Drehen** — `R`/`Shift+R` dreht um 90° im/gegen den Uhrzeigersinn, nur
  Ansicht; `0` setzt es zusammen mit dem Zoom zurück
- **Info-Overlay** — `I` schaltet eine Karte mit Dateiname, Position,
  Abmessungen, Dateigröße und Zoomstufe ein/aus
- **EXIF-Datenfenster** — `E`, oder der Link „EXIF-Daten anzeigen“ im
  Info-Overlay, öffnet Kamerahersteller/-modell, Objektiv, Belichtung,
  Blende, ISO, Brennweite und Aufnahmedatum für das aktuelle Bild (kein
  GPS/Standort)
- **Diaschau-Modus** — `P` schaltet eine Vollbild-Diaschau mit Überblendung
  zwischen den Bildern ein/aus; `↑`/`↓` stellen das (standardmäßig 10 s)
  Auto-Weiterschalt-Intervall ein, solange sie aktiv ist; `Shift+P` schaltet
  die Zufällige Wiedergabe ein/aus (`[Zufällig]` in der Titelzeile)
- **Kopieren** — `Cmd`/`Strg+C` kopiert das aktuelle Bild,
  `Cmd`/`Strg+Shift+C` kopiert seinen Dateipfad
- **Löschen** — `Shift+Delete` öffnet eine Bestätigungskarte (`←`/`→` zum
  Auswählen, `Return` zum Bestätigen, `Esc` zum Abbrechen); dauerhaft, kein
  Rückgängig
- **Handbuch** — `F1`, oder Hilfe -> Handbuch
- **Leeren / Beenden** — `Esc` (leert zuerst die geladenen Bilder, beendet
  dann; bricht stattdessen einen noch laufenden Ordner-Scan ab, falls einer
  läuft)
- **Formate** — JPEG, PNG, GIF (inkl. animiert), WebP, BMP, TIFF, ICO, XPM,
  HEIC/HEIF, AVIF
- **Maximale Fenstergröße** — 1500 × 950
