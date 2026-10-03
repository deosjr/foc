# Ideas: commanding through the fog

A brainstorm of features for Fog of Command, each grounded in how real
commanders dealt with slow, lossy and biased information. Sources were checked
on 3 October 2026; links point to readable translations or summaries.

The game already has the core of it: couriers with delay, interception,
generals who misread by temperament, distorted and incomplete reports,
conditional orders, and a review that shows the truth afterwards. The ideas
below deepen one of three things: **the channel** (how words travel), **the
report** (how much to trust what comes back), and **the general** (what he does
when the letters stop making sense).

## Recommended next steps

These fit the existing pipeline with modest work, and each adds a real
decision for the player rather than more noise.

### 1. Beacon signals: fast but mute

**History.** Polybius (10.43–47) describes fire-signal systems and criticises
the older water-vessel method because it could only send messages agreed in
advance, never unexpected news. He perfected a torch alphabet invented by
Cleoxenus and Democleitus. ([Polybius 10](https://penelope.uchicago.edu/Thayer/E/Roman/Texts/Polybius/10*.html))

**Mechanic.** Before the campaign, or by letter, the player agrees a handful
of signals with a general ("the enemy is at the ford", "fall back on Karsa").
Beacon chains on hills carry a signal the same turn, ignoring courier delay,
but only from the fixed list. Anything else still needs a letter. Beacons can
be seen by the enemy, and a beacon hill can be taken.

**Why it's interesting.** It forces the speed-versus-expressiveness trade-off
that defined ancient signalling, and it pairs naturally with conditional
orders ("if you see the fire on Duna Hills, march").

### 2. Send it twice, by different roads

**History.** In 427 BC Athens reversed its decree to massacre Mytilene. A
second trireme rowed day and night, the crew eating at the oars, to overtake
the first, and arrived as the order was about to be carried out
(Thucydides 3.49). ([Mytilenean revolt](https://en.wikipedia.org/wiki/Mytilenean_revolt))
In 207 BC Hasdrubal's six riders got lost and were captured, and his letter
gave away his plan (Livy 27.43). ([Livy 27](https://en.wikisource.org/wiki/From_the_Founding_of_the_City/Book_27))

**Mechanic.** The player chooses a letter's road when the map offers more than
one, and may send copies by several roads (a limited number of couriers per
turn). A copy that arrives later than another is read as a repeat, unless it
was a *countermand*, and then which arrives first matters. The route overlay
already draws roads; this makes them a choice.

### 3. A letter, or a messenger's word

**History.** Nicias wrote to Athens from Syracuse because he feared that
messengers would misreport "through inability to speak, failure of memory, or
a wish to please the multitude" (Thucydides 7.8). ([Thucydides 7](https://classics.mit.edu/Thucydides/pelopwar.7.seventh.html))

**Mechanic.** A general may send news by messenger instead of a sealed letter:
faster (a messenger rides light), but it passes through a second distortion
layer with the messenger's own small biases, and it reaches the player as a
retold summary rather than the general's words. The distortion code already
exists; this applies it twice.

### 4. Marching to the sound of the guns

**History.** At Waterloo, Gérard urged Grouchy to march toward the cannon
fire; Grouchy held to the Emperor's written order to pursue the Prussians.
([Grouchy's orders](https://www.napoleon-series.org/military-info/battles/1815/c_grouchyorders.html))
In 207 BC the consul Claudius Nero, having read Hasdrubal's intercepted letter,
marched north without the Senate's authority and won at the Metaurus
(Livy 27.43).

**Mechanic.** A general hears battles in neighbouring provinces. A high-
Initiative general may leave his orders to join one (support or attack), and
a low-Initiative one keeps to the letter, as Grouchy did. Either can be right.
The report then says why. This gives Initiative a role beyond unclear letters.

### 5. Captured letters cut both ways

**History.** Philip II wrote a false letter saying Thrace had revolted and
made sure the Athenians intercepted it, so they withdrew their fleet
(Frontinus, *Strategemata* 1.4.13). ([Frontinus 1](http://penelope.uchicago.edu/Thayer/E/Roman/Texts/Frontinus/Strategemata/1*.html))
Themistocles sent his slave Sicinnus to tell Xerxes the Greeks were about to
flee, drawing him into the straits at Salamis (Herodotus 8.75).
([Sicinnus](https://en.wikipedia.org/wiki/Sicinnus))

**Mechanic.** The enemy reads the letters it intercepts: the enemy AI turns
a captured dispatch into orders (a cheap call to the same decision model). The
player can then write a *decoy* and route it past the enemy on purpose. Cipher
(below) protects real letters at a cost. This is the spec's "symmetry"
extension, in its smallest form.

## Further ideas

**Secret writing.** Aeneas Tacticus lists ways to hide messages: letters sewn
into sandals, pin-pricks in an innocent text, vowels replaced by dots
(ch. 31). ([Aeneas Tacticus](https://topostext.org/work/98)) Sparta's scytale
carried enciphered dispatches, including the ephors' order recalling Lysander
(Plutarch, *Lysander* 19). ([Plutarch](https://topostext.org/work/167))
*Mechanic:* a letter marked "in cipher" tells an interceptor nothing, but the
general reads it a little worse (a lower confidence on every answer), so
ciphered orders drift toward the ambiguous branch.

**Relay stations.** Herodotus (8.98) describes the Persian *angareion*: one
man and one horse per day's stage, handing the message on like a torch race.
([Herodotus 8.98](https://scaife.perseus.org/reader/urn:cts:greekLit:tlg0016.tlg001.perseus-eng2:8.98.1))
*Mechanic:* an army that rests a turn on a road can leave a relay post that
halves courier delay through it, until the enemy burns it.

**Readback.** At Balaclava, Raglan could see the guns he meant and Lucan could
not. When Lucan asked which guns, Captain Nolan swept his arm toward the
wrong ones. ([Charge of the Light Brigade](https://en.wikipedia.org/wiki/Charge_of_the_Light_Brigade))
*Mechanic:* a letter can ask the general to repeat his reading back before
acting. That costs a round trip, but the player sees the misreading before it
happens. The spec already lists readback as a later extension.

**Commander's intent.** Moltke wrote that no plan survives first contact with
the main enemy force, and his directives favoured stating aims over detailed
instructions. ([Mission-type tactics](https://en.wikipedia.org/wiki/Mission-type_tactics))
*Mechanic:* each general keeps a one-line intent from the player ("keep the
enemy out of the Duna Hills"). His own-judgement step, now a fixed heuristic,
asks the decision model what serves that intent when letters are late or
unclear.

**More than one source.** Clausewitz: "Many intelligence reports in war are
contradictory; even more are false, and most are uncertain" (*On War* I.6).
([Clausewitz quotations](https://clausewitz.com/readings/Cquotations.htm))
Sun Tzu's five kinds of spies include the *doomed* spy, sent with false news
to be caught (ch. 13). ([Sun Tzu 13](https://standardebooks.org/ebooks/sun-tzu/the-art-of-war/lionel-giles/text/chapter-13))
*Mechanic:* merchants, deserters and spies as cheap, noisy extra reports; the
player must weigh them against the generals' letters. Pencilled guesses on the
map (an open question in the spec) make the weighing visible.

**The enemy deceives too.** At the Teutoburg Forest, a real revolt among
distant tribes was started on purpose to draw Varus out, while Arminius stayed
at his side; Segestes' warning was ignored (Cassius Dio 56.19; Velleius 2.118;
Tacitus, *Annals* 1.55). ([Cassius Dio](https://www.livius.org/sources/content/cassius-dio/cassius-dio-on-the-teutoburg-forest/))
*Mechanic:* the enemy can feign (extra campfires that inflate what generals
see) or send a false deserter whose story enters a report.

**Trust and politics.** After the victory at Arginusae, a storm prevented the
rescue of shipwrecked crews, and the Assembly executed six of the winning
generals after a dispute over their report (Xenophon, *Hellenica* 1.7).
([Arginusae](https://en.wikipedia.org/wiki/Battle_of_Arginusae)) Minucius
disobeyed Fabius, won a skirmish, convinced Rome that Fabius was timid, and
was given equal command before being trapped (Livy 22.24–30).
([Livy](https://www.livius.org/sources/content/livy/livy-periochae-21-25/))
*Mechanic:* generals report on each other, with rivalries; a successful
general's ambition grows; the player can recall one, and the recall takes
turns to arrive and may be ignored.

**News that arrives too late.** The Battle of New Orleans was fought on
8 January 1815, two weeks after the Treaty of Ghent was signed; the treaty
took effect only on ratification, and the news had not arrived.
([Battle of New Orleans](https://en.wikipedia.org/wiki/Battle_of_New_Orleans))
*Mechanic:* a truce scenario: the enemy offers terms, and the player's letters
accepting them must reach every general before someone fights on.

## How these would build on the current game

| Idea | Uses | New parts |
| --- | --- | --- |
| Beacons | conditional orders, routes | a signal list per general; beacon provinces |
| Copies by road | couriers, interception, route overlay | route choice in the composer |
| Messenger vs letter | distortion, report writer | a second distortion pass |
| Sound of the guns | perception, Initiative, policy | a step before standing orders |
| Captured letters | interception, decision model | enemy reads letters; decoy flag |
| Cipher | decision answers | a confidence penalty |
| Readback | clarification letters | a "read back first" flag |
| Commander's intent | own-judgement step | an intent field; a decision call |
