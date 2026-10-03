# Ideas: commanding through the fog

A brainstorm of features for Fog of Command, each grounded in how real
commanders dealt with slow, lossy and biased information. Sources were checked
on 3 October 2026; links point to readable translations or summaries.

Several ideas draw on Bret Devereaux's series *Total Generalship: Commanding
Pre-Modern Armies* on A Collection of Unmitigated Pedantry:
[I: Reports](https://acoup.blog/2022/05/27/collections-total-generalship-commanding-pre-modern-armies-part-i-reports/),
[II: Commands](https://acoup.blog/2022/06/03/collections-total-generalship-commanding-pre-modern-armies-part-ii-commands/),
[IIIa: Discipline](https://acoup.blog/2022/06/17/collections-total-generalship-commanding-pre-modern-armies-part-iiia-discipline/),
[IIIb: Officers](https://acoup.blog/2022/06/24/collections-total-generalship-commanding-pre-modern-armies-part-iiib-officers/),
[IIIc: Morale and Cohesion](https://acoup.blog/2022/07/01/collections-total-generalship-commanding-pre-modern-armies-part-iiic-morale-and-cohesion/).
Much of the series concerns the battlefield itself, below this game's scale;
what carries over is listed under "From *Total Generalship*" and folded into
the ideas above it. Its central claim fits the game's premise: the
constraints that defined ancient command are exactly the ones strategy games
usually remove for playability.

The game already has the core of it: couriers with delay, interception,
generals who misread by temperament, distorted and incomplete reports,
conditional orders, and a review that shows the truth afterwards. The ideas
below deepen one of three things: **the channel** (how words travel), **the
report** (how much to trust what comes back), and **the general** (what he does
when the letters stop making sense).

## Recommended next steps

These fit the existing pipeline with modest work, and each adds a real
decision for the player rather than more noise. Of the ideas taken from
*Total Generalship* below, "Silence is not safety" is the cheapest and
"Offering and refusing battle" the most promising; both could join this list.

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
orders ("if you see the fire on Duna Hills, march"). Devereaux (Part II) makes
the same point about trumpets and standards: they carried only a handful of
prearranged signals, and signal chains were an operational tool, not a
battlefield one, which is the scale this game plays at.

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
Devereaux (Part I) draws the contrast that matters: official messengers on
pre-arranged routes could cover over a hundred miles a day, while most news
travelled at the pace of traders and armies.
*Mechanic:* an army that rests a turn on a road can leave a relay post that
halves courier delay through it, until the enemy burns it. Off the relay
roads, letters keep the current slower pace.

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
Devereaux (Part I) lists the sources a general actually had: his own cavalry
scouts (reliable but short-ranged), traders on the roads, friendly locals and
political dissidents, deserters and prisoners. Reports were very often
wrong, and deliberate disinformation was ordinary practice.
*Mechanic:* merchants, locals, deserters and spies as cheap, noisy extra
reports, each with its own reliability and speed (rumour moves at a trader's
pace); the player must weigh them against the generals' letters. Pencilled
guesses on the map (an open question in the spec) make the weighing visible.

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

## From *Total Generalship*

### Silence is not safety

**Source.** Part I stresses that the absence of reports did not mean a region
was safe, only that the general was blind there, and that he never knew
whether his picture was complete.

**Mechanic.** The believed map should tell the two apart. Today a province
with no enemy marked looks the same whether a general saw it empty last turn
or nobody has looked for ten turns; the "T5" badge is easy to miss. Shade
never-seen and long-unseen provinces as unknown, and show "seen clear, T5"
explicitly. This is small, sits entirely in the web layer, and makes the fog
legible rather than just present.

### Mistaking a detachment for an army

**Source.** At Delium (424 BC) Athenians took a cavalry detachment for a
second army and broke (Thucydides 4.96); at Tifernum (297 BC) Fabius made his
infantry appear as a fresh army to panic the enemy (Livy 10.14). Part I notes
that overall size was the one thing that was hard to hide, while details were
easy to get wrong.

**Mechanic.** Keep the size noise as it is, but add occasional gross errors
in *what* was seen: a small force reported as a host, two armies merged into
one, or an army seen where only its foragers were. An enemy "feint" order
could raise the chance deliberately, which gives the enemy-deception idea a
concrete form.

### Offering and refusing battle

**Source.** Part I describes armies camped a few miles apart for days, each
drawing up on favourable ground and declining to fight on bad ground: the
armies at Mantinea (418 BC) formed and re-formed over several days
(Thucydides 5.64ff), and at Philippi (42 BC) the armies faced each other for
weeks before the battle. Neither side at Cynocephalae (197 BC) knew exactly
where the other was, and the battle began as an encounter in fog
(Polybius 18.18–21).

**Mechanic.** Battle should usually need two willing sides. An army ordered to
avoid battle (a cautious general, or a letter with a low engagement score)
camps on strong ground and refuses: an attacker must assault the camp at a
heavy penalty, or wait, or go round. Two armies blundering into the same
province without a chance to choose fight an *encounter* battle, where
terrain bonuses don't apply. The letter's engagement score, now used only for
reweighting ambiguous orders, would then mean something in every battle.

### Generals can fall

**Source.** Part IIIc: armies were held together by morale and cohesion, and
the death of the leader could end a battle at once. When Cyrus the Younger
fell at Cunaxa (401 BC) victory was no longer possible for his army, and only
the Greek mercenaries held together (Xenophon, *Anabasis* 1.8). At Hastings,
William had to bare his head to show he was alive. Part II describes how
generals led from the front or moved between threatened points, and how
soldiers expected a culturally fitting kind of courage of them.

**Mechanic.** A general who leads from the front (high Aggression and Vanity:
Velk) wins more of his close fights, but risks being killed or captured when
he loses one. His army then falls to a second-in-command with an unknown
temperament and a new voice in the letters. The player may write to a dead man
for turns before the news arrives, and a *false* report of a general's death
(an enemy rumour) is a natural deception.

### Morale breaks armies, not losses

**Source.** Part IIIc: pre-modern armies usually broke long before heavy
losses. Devereaux cites averages of about 5% casualties for winners and 14%
for losers in Greek hoplite battles, and similar figures for Roman battles,
with most of the loser's dead falling in the pursuit. Morale and cohesion can
also fail separately: an army may keep its ranks yet refuse to advance, as
French divisions did in 1917.

**Mechanic.** Give each army a morale value that falls with defeats, with
long marches without rest and with the death of its general, and rises with
victories and rest. Low morale makes an army refuse attacks on its own, apart
from its general's loyalty, and a general may or may not admit it (Honesty).
Casualties would move toward the historical pattern (loser ~15%, winner ~5%,
more in a rout), and that needs re-tuning with `foc sim`, since lower losses
make stalemates likelier.

### What an army can actually do

**Source.** Part IIIa: an army could only carry out what it had drilled, a
limited "menu" of manoeuvres, and adding to it took training; Caesar drilled
his men against elephants before Thapsus (*Bellum Africanum* 84). Part IIIb:
how much an army could adapt depended on officers trusted to act on their own,
as at Cynocephalae, where a tribune, not the general, turned maniples into the
Macedonian flank (Polybius 18.26).

**Mechanic.** A *drill* or quality value per army. Veterans carry out
supports, scouting and conditional orders reliably. Fresh winter levies (the
new musters) dilute it, so complex orders sometimes fail ("the levies could
not be brought up in time"), and a won battle may end in pursuit and plunder
rather than the next move. Armies that rest regain it. This puts a cost on
the musters and makes "which army do I trust with the hard job" a real
question.

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
| Silence is not safety | belief map, web map | unknown versus seen-clear shading |
| Misidentified forces | perception noise | gross-error draws; enemy feints |
| Offering and refusing battle | engagement score, entrench | a refuse-battle stance; encounter battles |
| Generals can fall | battles, reports, letters | general death and succession; rumours |
| Morale | battles, refusals, honesty | a morale value per army; re-tuned casualties |
| Drill | musters, supports, conditionals | a quality value per army |
