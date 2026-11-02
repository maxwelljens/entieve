# Research Brief: Skills and Tech Trees, Active vs. Passive

**Question:** How do games implement skills and tech trees, and which designs are *active* (growth
requires doing something in play) rather than *passive* (growth accrues on its own)? The owner of
Entieve likes EVE Online's simple passive real-time skill training but wants something more active,
so every candidate is weighed against Entieve's constraints and the systems the game already has.

**Scope decisions taken (assumptions):** no repository paths were given beyond the brief, so findings
are saved at `docs/research/skills-tech-trees-brief.md`, beside `price-formation-brief.md`, whose
format this file matches (title, **Question:**, **Scope decisions taken (assumptions):**, **Fact vs.
inference:**, numbered sections of quoted primary evidence with URLs, then `## Candidate mechanisms`
and `## Recommended operationalization`, then a source list). Constraints, read from the repo: the sim
steps `sim.TickRate` = 60 ticks/s and RNG exists only in role-pinned, test-pinned jobs, so any
mechanic must be a pure function of tick + state; wall-clock training is meaningless in a pausable
single-player game; content is data-driven JSON that must pass `just validate`; CONTEXT.md's rule
"New concepts get named here before they get built" means every mechanism below proposes a *name*
flagged as a proposal; and candidates must interlock with one system the game already has (ship
fitting, industry, the war front, the ledger-agent market, derived mission offers, the standing
scalar) rather than float above them. The brief's "+5% per level" EVE figure was treated as a claim
to source. Every quoted line was re-retrieved from its source URL while writing (official sources
where reachable; PDFs via `pdftotext`; a 2021 dev log and a 2018 forum reply via Wayback). Where no
primary source was reachable I mark the item unsourced or drop it.

**Fact vs. inference:** everything under sections 1 to 6 is quoted from a cited source, with source
type named (official studio wiki/manual/dev blog/patch note/studio forum = primary; community wiki =
secondary, labelled as such). Everything under "Candidate mechanisms" and "Recommended
operationalization", plus any sentence marked *(inference)* or *(opinion)*, is my synthesis.

---

## 1. EVE Online: the passive baseline the owner likes

### 1.1 Real time, logged off, from a queue, and a gate on gear

Official support article *Skill Training* (support.eveonline.com, updated 2024):

> "Most items such as ships, modules, ammunition and even Skills will require some skills to be
> trained to a certain level before they can be used."
> "The Skill training itself happens in real time, independently of the player being logged into the
> game. . To cover offline times, up to 150 skills may be queued up into the training queue."
> "The speed at which a skill accrues skill points while it is training is determined by two out of
> the 5 attributes of the character. ... The formula for skill training on an Alpha Clone is:
> `0.5 * (Primary Attribute + (Secondary Attribute/2)) = SP per minute`. An Omega Clone will train at
> double the speed."
> "the skill training speed can be increased with Neural Remaps ... Implants and Cerebral
> Accelerators can also be used to shorten skill training time."
> https://support.eveonline.com/hc/en-us/articles/203217062-Skill-Training

CCP calls it the core system and lengthened the queue: "Skills can be seen as the primary progression
system in EVE Online"; "The current limit of 50 skill entries in the queue will be increased to 150
for both Alpha and Omega clones." (*Updates To Skill Training*, 2021,
https://www.eveonline.com/news/view/updates-to-skill-training)

So EVE is two designs at once: skills **gate content** (the item is unusable below the required
level) and skills **grant edges** (per-level percentages). That split is the question the synthesis
at the end has to answer for Entieve.

### 1.2 What a level costs (community reference, secondary)

EVE University Wiki, *Skills and learning* (community wiki; no official page publishes the ladder):

> "The number of skill points necessary to train a skill is determined by the skill's rank, also
> known as its training time multiplier, while the rate at which you get new skill points is
> determined by your attributes." Totals for a 1x skill by level: "I 250 / II 1,414 / III 8,000 /
> IV 45,255 / V 256,000".
> "there were 494 different skills in EVE ... and a total of 604,416,000 skill points. Every Omega
> clone character can potentially learn every skill, but it would take over 24 years of skill
> training even with 32 points at all attributes".
> https://wiki.eveuniversity.org/Skills_and_learning

*(Inference)*: level V is ~82% of a 1x skill's total cost, so I to IV are the reachable middle and V
a long tail; the rate is a pure function of two character numbers, a shape a pausable game can
reimplement against its tick clock.

### 1.3 What a level buys: +5% is real, but per-skill

Official patch notes: "Skill (Armor Resistance Phasing) now reduces cap need by 5% per level in
addition to reducing cycle time." (*Patch Notes for Retribution*,
https://www.eveonline.com/news/view/patch-notes-for-retribution-1) Community reference, same idiom:
"Power Grid Management increases a ship's Power Grid output by 5% per level." (EVE University,
*Fitting skills*, https://wiki.eveuniversity.org/Fitting_skills)

*(Inference)*: no official source states a universal +5%-per-level law; bonus sizes are authored per
skill and 5% is one of them (no official table found, not sourced here).

### 1.4 Injectors: CCP's own words on the pay-to-win surface (primary)

*Skill Trading in New Eden* (CCP Rise, 2016-01-18): a Skill Injector "immediately adds skillpoints
to their unallocated pool", at a diminishing scale: "< 5 million total skillpoints = 500,000
skillpoints per injector / 5 million – 50 million = 400,000 / 50 million – 80 million = 300,000 /
> 80 million total skillpoints = 150k skillpoints per injector"; extraction takes 500,000 SP and a
Skill Extractor. CCP on the contract and the reaction:

> "all the skillpoints available to buy on the market in EVE will have originated on other characters
> where they were trained at the normal rate. ... there is one single mechanism that brings new
> skillpoints in to the system – training."
> "Rather than seeing major concerns about this being used by high skillpoint characters we saw a lot
> of high skillpoint players who wanted better access to using it themselves and so we've relaxed the
> scale a little".
> https://www.eveonline.com/news/view/skill-trading-in-new-eden

---

## 2. Learn by doing: growth that requires the action itself

### 2.1 Ultima Online, the origin of the model (developer's own writing)

Raph Koster (UO's system designer), *Use-based systems* (2006):

> "The basic definition of a use-based skill system is one where you have a chance of improvement
> every time you use the skill. ... as you exercise the skill, your chance of success goes up."
> "UO used a straight-up use-based system. Each time you used a skill, it had a chance of going up."
> https://www.raphkoster.com/2006/07/18/use-based-systems/

Koster names the failure modes in the same essay: cheese ("people jumping everywhere in order to
improve the stats and skills related to dodging") and, in UO, decay plus a ceiling ("A player was
allowed only 700 skill points total"). *(Inference)*: grind-cheese and decay are the original pair.

### 2.2 Bethesda: use the skill, raise the level, take a perk (official manuals)

*Skyrim* official manual, "Increasing Your Skills and Leveling Up":

> "Use a skill repeatedly. The lower the skill, the faster it increases from use. The higher the
> skill, the longer it takes to increase."
> "To raise your level, increase any combination of your skills multiple times. ... When you level
> up, you must choose whether to increase your Health, Magicka, or Stamina. You also gain one Perk
> selection."
> https://assets.ctfassets.net/rporu91m20dc/hH1HljyrjqewisSugMOm2/ab667e857bd67d39a814dc064d96ef8f/manual_skyrim-le_ps3_en-int.pdf

*Morrowind* official manual:

> "To increase a skill, you simply need to use it successfully several times. The lower your skill,
> the harder it is to use, but the number of uses you need to increase it is smaller."
> https://assets.ctfassets.net/rporu91m20dc/6lHLSvBn8WoeCou2i0IAqa/77f3c4493ad458ab5a692d593f1af8a7/manual_mwgoty_pc_en-us.pdf

*(Inference)*: self-balancing on paper, but it rewards training your weakest skill and never resets:
the "expertise forever" model. Skyrim's one-Perk-per-level is the other half: a narrow action gate
feeding a broad pool the player spends deliberately.

### 2.3 Kingdom Come, Kenshi, Dwarf Fortress (primary, with labels)

Kingdom Come, in-game Codex text (Warhorse's words, read through the community codex reader):

> "As well as [Character Stats] you can improve your skills. This is done by using them repeatedly
> and there is nothing to stop you from developing them to the maximum."
> "After your third level up of each Skill, you will be given the option to choose a Perk that in
> some way improves or alters ..."
> https://kingdomcomecodex.github.io/ (raw game language file:
> https://raw.githubusercontent.com/kingdomcomecodex/kingdomcomecodex.github.io/master/lang/en.html)
> *(the file is garbled at the next word, literally "the e of the given skill"; "[behaviour]" is a
> reconstruction, not a verified quote. The "faster learning at higher use" wording often attributed
> to KCD is NOT sourced here, secondary only.)*

Kenshi, official studio forum, posted by a Lo-Fi developer ("Captain Deathbeard", rank Developer):

> "Its skill based, so they increase as you go, there is no overall level. Skills increase with use,
> but on top of that you also build up skill points that can be manually distributed."
> https://www.lofigames.com/phpBB3/viewtopic.php?p=429
> *(2008 pre-release answer; "manually distributed" should not be read as shipped behaviour.)*

Dwarf Fortress, official Bay 12 dev log (entry 03/19/2021):

> "Finally, the distance of the practice shot matters for skill gain, and the existence of the target
> doubles skill gain. So if you make a 2x1 zone where the dwarf just shoots at the wall, the dwarves
> will train at about 1/10th the speed of a dwarf in a 12x1 range with a target at the end."
> https://www.bay12games.com/dwarves/
> *(the "dwarves gain skill as they perform jobs" rule exists only on the community wiki
> dwarffortresswiki.org; no official Bay 12 manual exists, so it stays secondary.)*

*(Inference)*: the line worth copying is DF's: the quality and distance of practice scales the gain,
not just repetition. How you train is the cheapest anti-cheese tool in this family.

### 2.4 Mount & Blade II: Bannerlord: use raises the skill, committed points gate it (official dev blog)

Official TaleWorlds *Dev Blog 23/08/18*, read from the Wayback copy of the official URL (direct
fetches get 403):

> "To improve these skills you need to perform the associated task or action."
> "Once the bar is completely filled, your character will level up and you will be rewarded with two
> extra focus points. ... Skill focus allows your character to gain experience at a faster rate for
> that particular skill. However, once you invest a skill point into a focus then it is permanently
> locked in. To gain skill points in any of the skills, you need to have made an initial investment of
> focus points, meaning that skills that are shown as level 0 and have no focus points invested in
> them cannot progress, no matter how much you practice them."
> "Players are unable to unlock all of the perks, and in many cases, a choice will have to be made
> about which perks that you would like to choose."
> "All of this applies to companions in your party too, which players have total control over."
> https://www.taleworlds.com/en/Games/Bannerlord/Blog/74 (snapshot:
> https://web.archive.org/web/2021id_/https://www.taleworlds.com/en/Games/Bannerlord/Blog/74)

*(Inference)*: the strongest anti-grind shape here: action raises the skill, only *committed* points
let it rise, commitment is irreversible, and the perk pool is smaller than a full clear.

### 2.5 X4: crew learn by doing while you do something else (primary, studio wiki)

Official Egosoft-hosted X Community Wiki, *Crew*:

> "### Learning by Doing
> Everyone gets better the longer they survive! While marines best enhance their main attribute
> 'boarding' on real boarding operations, 'normal' crewmembers learn from usual ship actions.
> However, surviving a fight improves their skill across many areas!"
> "The quality of employees is summarized in capability groups. These abilities are displayed with a
> star rating of 0 to 5 stars."
> https://wiki.egosoft.com/X4%20Foundations%20Wiki/Manual%20and%20Guides/Objects%20in%20the%20Game%20Universe/Equipment/Crew/

### 2.6 Entieve already ships a learn-by-doing skill (this repo)

`CONTEXT.md`: "**Craft level** | a station's learned skill per product family (grows with volume)".
`sim/system_industry.go`:

> "// CraftLevel is a station's craft level in a recipe family:
> L = min(craftMax, 1 + floor(sqrt(totalRuns)/2))
> so L1 needs 0 runs, L2 4, L3 16, L4 36 and L5 64 (the cap binds from 100 runs on, where the raw
> formula would read L6)."

`docs/SIM.md` §3.3: "Craft grows per (station, recipe family): `L = min(5, 1 + floor(sqrt(runs)/2))`,
each level cutting job time 4 % and materials 2 % below the L1 baseline (both frozen at job start).
Every 3600 idle ticks a family's counter decays one run-equivalent, floor 0".

*(Inference)*: the house style is already approved: action-count in, percentage out, sqrt pacing, cap,
idle decay, integers. Any new skill system competes with this precedent.

---

## 3. Adjacent systems: daily caps, limited points, paid skips

### 3.1 Warframe Focus: earn actively, capped daily on the wall clock (primary, official wiki)

> "Focus Points have a daily limit of 250,000, which scales at the rate of 5,000 additional cap per
> Mastery Rank. This limit resets at 00:00 UTC."
> "Convergence Orbs ... multiply the amount of Affinity converted to Focus by 10x for either a period
> of 45 seconds, until death, or when the daily Focus cap has been reached, whichever is shortest."
> https://wiki.warframe.com/w/Focus

*(Inference)*: the cap turns play into a daily quota, hard-dependent on UTC, which a pausable
single-player game cannot borrow.

### 3.2 Guild Wars 2 hero points: buy unlocks with earned points, then re-price them (primary, official studio news)

ArenaNet, *Specializations, Part Two* (2015):

> "everything will be unlocked by spending Hero Points on these new reward tracks."
> "Hero Points will be limited, and they'll be earned strictly through what are currently called
> skill challenges (these will become known as hero challenges) and leveling up. A level 80 character
> that's done none of the hero challenges should be able to unlock more than enough skills,
> specializations, and traits to make several unique full builds."
> https://www.guildwars2.com/en/news/specializations-part-two-reward-tracks-and-elite-specializations/

ArenaNet, *Heads Up! Iteration Incoming* (2015), after feedback:

> "reducing the cost of unlocking every possible ability for an elite specialization from 400 hero
> points to 250 hero points total. ... If you've already put more than 250 points into your elite
> specialization, surplus hero points will be refunded to your character for future use."
> https://www.guildwars2.com/en/news/heads-up-iteration-incoming/

*(Inference)*: the cleanest "currency for unlocks" here, with documented evidence that first-pass
prices land too slow, and a studio absorbing the re-price with refunds instead of blocking respec.

### 3.3 FFXIV: the paid skip (primary, official store page)

> "Instantly level one job of your choosing to level 90, complete with a set of gear to begin your
> adventure." "Job Actions | All actions granted through job quests up to level 90 will be acquired
> automatically."
> https://na.finalfantasyxiv.com/tales_of_adventure/

*(Inference)*: the home loop is ordinary XP from play (not sourced here) and the store sells arrival.
Either way: long action-gated climbs eventually get a purchasable exit, which is exactly the pressure
a passive system never creates (§6.6).

---

## 4. Active tech trees: you choose, you feed, you wait

### 4.1 Factorio: research is a production order your factory must fill (primary, official wiki)

> "Research is performed by labs consuming different types of science packs."
> "In singleplayer games, only one technology can be researched at a time."
> "Research progress for a technology is divided into *units*. Each unit has a time and a science pack
> cost. When research is in progress, labs with the required amount of science packs for one unit
> will consume the amount and contribute towards research progress."
> "`T = (T[0] × P) ÷ (L × S)` seconds" (time per unit × price in units, over labs × lab speed).
> "Science pack production automation is the primary incentive for factory development throughout the
> game."
> https://wiki.factorio.com/Research

*(Inference)*: research pace *is* the player's production function; no timer exists that the factory
does not supply. The strongest template for a game with working industry already.

### 4.2 Anno 1800: population generates the currency, completion still takes clock time (primary, official dev blog)

> "your scholars will get to work and generate a constant stream of research points. The more Scholars
> you have attracted, the faster they accumulate research points and the more research points you can
> 'store'."
> "Another way of acquiring research points is by donating items – for science! The amount of points
> received depends on the rarity of the respective item."
> "Researching items and especially Major Discoveries takes time ... To speed things up, you can
> assign parts of your Engineer workforce – the Scholars' tutors."
> https://www.anno-union.com/devblog-scholars-and-research/

*(Inference)*: item donation turns existing inventory into research velocity with no new resource
type.

### 4.3 Civilization VI: doing the thing halves the cost, once (primary, official 2K manual)

> "The village provides your civilization with a free technology ... a technology boost (50% of the
> needed research towards a technology), or even multiple technology boosts."
> "clearing the outpost will help you work towards the 'boost' for the Bronze Working technology,
> making it faster to research."
> "These tech boosts will not stack with the ones earned from normal play."
> http://downloads.2kgames.com/civilization/vi/manuals/eu/CIV_VI_25TH_ONLINE_MANUAL_ENG.pdf

**Conflicting evidence (flagged, not resolved):** the official manual says 50%; community sources
assume the live value is 40% ("Eurekas and Inspirations provide 50% of civics and technologies
instead of 40%", Better Balanced Game changelog, https://civ6bbg.github.io/, and
https://civilization.fandom.com/wiki/Boost_(Civ6)). *(Inference)*: the mechanism (do the thing, cut
the cost roughly in half, once, no stacking) is agreed by both; the number is version-dependent.

### 4.4 Stellaris: a weighted card hand instead of a tree (primary, official Paradox wiki)

> "The user interaction aspect utilizes a *card shuffle* approach rather than a traditional *tech
> tree* presentation, thereby introducing an element of semi-randomness into the system."
> "Higher tier technologies require obtaining a certain number of technologies from the preceding
> tier in the same area of research (set at 6 for all tiers) before they become available as research
> alternatives."
> "*Weight* – A base value denoting the *relative* chance (out of all the valid options) of a
> particular technology to be drawn as an alternative. ... a technology which appeared as an
> alternative in the previous card hand has its weight halved in the next draw".
> https://stellaris.paradoxwikis.com/Technology

*(Inference)*: the anti-template for Entieve: the interesting decision (which tech) is replaced by a
draw. Its one transferable idea is deterministic and cheap: gate higher tiers behind a count of
lower-tier picks, not a fixed tree.

### 4.5 Master of Orion II: pick a direction, refunds on abandonment (primary, official manual scan)

> "At the end of the first turn, your Research Director invites you into the lab and asks you in what
> direction you want to focus your race's research efforts. You get to pick from several categories,
> in each of which one field is listed."
> "To successfully research a field, your empire must accumulate a certain amount of Research Points
> (RPs). Population assigned to science and some improvements you can make to colonies generate RPs
> each turn."
> "When the amount of RPs you've accumulated passes the Base Cost of an advance ... there is a good
> chance every turn that your scientists will make the breakthrough. (Under no circumstances will any
> research cost more than twice the listed RPs.)"
> "You can click on this indicator if you decide to change the direction of their labours. (If you do
> so, all the points spent toward the former research goal are miraculously applied to the new goal.)"
> https://archive.org/details/Master.of.Orion.2 (official manual scan inside the item)

*(Inference)*: abandoned goals are refunded (no respec cost), RP comes from population (research
pace = economic pace), and the completion tail is random with a price cap (the one bit of RNG a
deterministic sim must replace with tick math).

---

## 5. Perk trees: points earned by play, spent on gates

### 5.1 Path of Exile: one shared tree, points from play, refunds that stop short of rerolling (official site)

> "This is Path of Exile's passive skill tree. It is a vast web of 1325 skills ... Each time you level
> up or complete certain quests, you can allocate a skill and explore deeper into the tree. All
> character classes share the same tree, but start at different locations".
> "If you want to adjust your passive build during gameplay, there are Passive Skill Refund points
> available from quests or relatively rare items in the game that can be found or traded for. ...
> our intention is that players who want to try substantially divergent character builds are
> encouraged to play a new character through the game, organically leveling it up rather than just
> respeccing into it."
> https://www.pathofexile.com/passive-skill-tree

*(Inference)*: cheap *correction*, expensive *identity* is a pacing rule you can copy. The per-point
cost (Orb of Regret = one refund point) is community-wiki only (not sourced here).

### 5.2 Borderlands: rows gated by points already spent (official 2K pages)

> "Every Vault Hunter has three skill trees ... You'll need to invest five of the Skill Points you get
> from leveling up into a given row to unlock access to the next row. Once you've invested 15 total
> Skill Points into a skill tree, you'll have access to three Branches".
> https://borderlands.2k.com/borderlands-4/game-info/vault-hunters/vex/
> BL3 official patch notes confirm the same class of gate without the number: "If a player does not
> have enough points invested to unlock a high-tier skill they have previously applied points to,
> they will need to re-apply the refunded skill points before using those high-tier skills."
> https://borderlands.2k.com/borderlands-3/update-notes/

*(Inference)*: narrow growth, committed points, mistakes undoable: the tree disciplines decisions
without punishing experimentation. The "5 points" figure is official for Borderlands 4 only.

### 5.3 Hades: currency only from runs, a meta tree revealed in rows (official patch notes)

> "Removed ability to subtract ranks in a given talent; added a full Darkness refund option"
> "Talents are no longer all visible at the start of the game; they are now unlocked two rows at a
> time"
> "Later Talents cost less Darkness; if you had more invested than is now required, it has been
> refunded."
> https://www.supergiantgames.com/blog/hades-the-nighty-night-update-patch-notes/
> Darkness is a run reward: "Eclipse (Chaos): now also boosts Darkness drops from boss foes"
> (https://www.supergiantgames.com/blog/7/); the endgame knob is the "Pact of Punishment"
> (https://www.supergiantgames.com/blog/hades-faq/).

*(Inference)*: the cleanest active meta progression: you must run to earn, rows gate the tree, and
the studio shipped a full refund plus automatic reimbursement when prices fell: three re-pricing
moves in one note, evidence that meta trees get tuned late, by feel.

### 5.4 Not sourced for this cluster (dropped)

- **FTL crew XP by use**: official manual behind bot protection, no Wayback snapshot; only the
  community wiki states it. Not used as evidence.
- **Slay the Spire relics**: no Mega Crit page documents relic mechanics; only the press-kit line
  "discover relics of immense power" (https://www.megacrit.com/press-kits/slay-the-spire/). Not used.
- **WoW / Diablo II talent points**: no official page retrieved; dropped. (WoW's official Conduit
  Energy pages *are* sourced, in §6.5, but they document a different system.)

---

## 6. Anti-patterns: pacing and monetization failures, in the studios' own words

### 6.1 A hidden pacing knob reads as a lie

Bungie, *The State of Destiny 2* (2017-11-29):

> "we disabled a scaling mechanism that adjusted XP gains up and down without reflecting those
> adjustments in the UI. Our intention was to keep slower-paced activities as rewarding as high
> intensity grinding ... but the silent nature of the mechanic betrayed the expectation of
> transparency that you have for Destiny 2."
> https://www.bungie.net/7/en/News/article/46504

*(Inference)*: the sin was the silence, not the curve; any Entieve curve must be readable in the UI
and published in the content, which a data-driven game gives for free.

### 6.2 The injector arc: caution, launch, then real money (official CCP dev blogs)

CCP, *Exploring the Character Bazaar: Skill Trading* (2015-10-15): "There is a lot to be careful
about here though ... The skill system is a fundamental part of EVE so we need to make sure we
account for all outcomes, which is why we're talking to you now."
https://www.eveonline.com/news/view/exploring-the-character-bazaar-skill-trading

CCP, *New Alpha Training Option* (2017-11-14), on the Daily Alpha Injector: "Can be purchased in the
NES for PLEX or purchased for your regions real money currency via secure.eveonline.com"
https://www.eveonline.com/news/view/new-alpha-training-option

*(Inference)*: the sequence to fear: a time-gated system whose backlog becomes a product. Note whose
feedback moved the design (§1.4): the incumbents, not new players.

### 6.3 The publisher reversed itself: progression earnable in play only

DICE/EA, *An Update on Star Wars Battlefront II*:

> "We hear you loud and clear, so we're turning off all in-game purchases. ... This means that the
> option to purchase crystals in the game is now offline, and all progression will be earned through
> gameplay."
> https://www.ea.com/games/starwars/battlefront/news/pre-launch-update

The defence that preceded it (EACommunityTeam staff account, Reddit; Wayback-verified): "The intent
is to provide players with a sense of pride and accomplishment for unlocking different heroes. ... we
selected initial values based upon data from the Open Beta..."
https://www.reddit.com/r/StarWarsBattlefront/comments/7cff0b/seriously_i_paid_80_to_have_vader_locked/dppum98/

*(Inference)*: pacing set "from data" rather than from a design contract is what got reversed. Write
it down first: growth comes from play; time and money never substitute for it.

### 6.4 Treadmill fatigue: two studios retracted their own grinds

Bungie, *The Road to The Witch Queen* (2021-02-25): "With Season 11, we introduced infusion caps ...
While we still believe in these goals, it's clear our execution was off the mark." "the system has
also made our rewards feel like they have an expiration date and have frequently made playing our
legacy content feel shallow." https://www.bungie.net/7/en/News/article/50124

Monolith/WB, *Free Updates Coming to Shadow of War* (2018-04-02): "we have come to realize that
providing this choice risked undermining the heart of our game, the Nemesis System. ... we'll be
permanently removing Gold, War Chests and the Market from Shadow of War."
https://wbgamessupport.wbgames.com/hc/en-us/articles/1500002839382-Important-Free-Updates-Coming-to-Shadow-of-War-FAQ-04-02-2018

Plus ArenaNet cutting an unlock price and refunding the difference (§3.2).

*(Inference)*: all three reversals share one shape: rewards that expire, or a core loop that became
optional, got walked back. The Entieve analogue: never make the player's existing toys obsolete;
edge them instead.

### 6.5 A recharge-time gate, removed by the studio in its own words (official Blizzard pages)

Blizzard, *9.1.5 Content Update Preview* (2021-08-27): "Conduit Energy is being removed and Conduits
will be freely swappable without any restrictions."
https://worldofwarcraft.blizzard.com/en-us/news/23716422

Blizzard, *World of Warcraft News and Development Updates* (2021-09-14), on the interim hotfix:
"This update will usher in the complete removal of the Conduit Energy system, but in the meantime,
we've rolled out a hotfix to change the refill rate of Conduit Energy from one per day to 10. This
change makes it easier for players to swap Conduits more often." and "including a change to give you
more Conduit Energy daily, along with much more."
https://worldofwarcraft.blizzard.com/en-us/news/23724371

Official *9.1.5 Content Update Notes* (2021-11-01) confirm it shipped: "Conduit Energy has been
removed and Conduits can now be freely swappable without any restrictions."
https://worldofwarcraft.blizzard.com/en-us/news/23731242

*(Inference)*: a refill-time currency on re-gearing; the studio raised the daily refill from 1 to 10,
then deleted the system. Any currency that refills on a wall clock reads as friction on *using* your
build, and it is the first thing a studio removes under pressure. A deterministic single-player game
never gets that pressure, and never needs the gate.

### 6.6 Why studios sell acceleration (studio statement, secondary host)

A Ubisoft representative on Assassin's Creed Valhalla's XP boosts (words Ubisoft's, host Game
Informer): "Utilities allow players who lack the time to fully explore the world of Assassin's Creed
Valhalla to be able to acquire the game's best gear ... by accelerating their progress."
https://gameinformer.com/2020/12/16/assassins-creed-valhalla-sells-xp-boosts-now

*(Inference)*: the stated reason is never greed, it is "players lack time". Passive training never
demands play; active training creates it. That is the honest cost of choosing active.

### 6.7 Not sourced (dropped)

Assassin's Creed Odyssey boost rationale (no primary interview found), Diablo Immortal / Genshin
official statements on time gates (none found), WoW level squish and grind postmortems (not
researched to quote level). None of these are asserted anywhere in this brief.

---

## Candidate mechanisms *(inference / opinion)*

### Taxonomy

**Active** = the player's gameplay act is the input to growth. **Passive** = growth accrues without a
per-act input (background, offline, or someone else acting). **Time-gated** = a clock or production
rate fills the bar. **Action-gated** = discrete acts fill the bar. *Broad* spans many activities or
the whole character; *narrow* is one skill/family. Which game lands in which quadrant is my
classification; the rule inside each cell is the source's.

| Quadrant (earning x gate) | Broad-spectrum example | Narrow example | Respec / reset rule |
|---|---|---|---|
| **Passive x time** (background accrual) | EVE Online: 0.5*(P+S/2) SP/min while logged off (§1.1) | Anno 1800: one Institute project advances on the clock once fed (§4.2) | EVE: extraction costs an item and 500,000 SP (§1.4) |
| **Passive x action** (an actor acts; player is not the per-act driver) | X4 crew: "Everyone gets better the longer they survive!" (§2.5) | Dwarf Fortress labor skills; official dev log shows gain scales with how the job is done (§2.3) | Bannerlord companions, same rule as the player: focus "permanently locked in" (§2.4); X4 and DF document no reset (gap) |
| **Active x time/production** (player triggers, a rate fills it) | Factorio: labs consume packs; `T = (T0 x P) / (L x S)` (§4.1) | Anno donation/queue then wait (§4.2) | MOO2: switching goals applies spent RP to the new goal (§4.5) |
| **Active x action** (each act is the input) | Skyrim: "Use a skill repeatedly..." → level → perk (§2.2) | Civ VI: one non-stacking ~50% boost per tech from doing the thing (§4.3) | PoE refund points from quests/rare items (§5.1); Hades full Darkness refund (§5.3) |

Every cell carries a sourced game; the one parenthetical gap (no reset documented for X4 or Dwarf
Fortress) is a real gap, not an omission. *(Opinion)* the designs players call "active" are all in
the bottom row, while the designs studios can monetize are all in the top row (§3.3, §6.2, §6.6).

### Candidates for Entieve

All four assume: no wall clock (ticks only), no new RNG (counters and formulas, like craft), content
defined in `content/data` with cross-validation, and a CONTEXT.md name before any code. Names below
are **proposals** (flagged for CONTEXT.md, not existing terms).

**A. Proficiency (proposal: "Proficiency", per module family or hull family).** The active twin of
EVE's % bonuses.
- **Loop:** fit a weapon/mining family and use it (shots, ore, deliveries) -> persisted integer
  counter -> level grants a small % edge *only while that family is fitted*.
- **Determinism:** counters ride `Save` in canonical order like `Save.Craft`; the bonus is pure
  arithmetic, frozen when craft freezes its factors; no RNG, no clock, twin hashes unaffected.
- **Interlock:** **fitting** (class quotas make "which family to master" a fitting decision) and
  **industry** (the craft precedent: sqrt curve, cap 5, 4%/2% per level, idle decay).
- **Pacing:** fast first levels, long expertise tail: EVE's cheap I-IV and expensive V without the
  wall-clock wait (§1.2).
- **Failure mode inherited:** grind-cheese (Skyrim/UO/Kenshi, §2.1-2.2; Skyrim rewards training your
  *weakest* skill) and opacity about what raises what (X4); the known fixes are committed points
  (Bannerlord, §2.4) or quality of practice (DF, §2.3).

**B. Field notes (proposal: "Field note").** Civ VI Eurekas as derived state, not checklists.
- **Loop:** perform a specific world act (first kill of a caste at a belt, a delivery landed, 100 ore
  refined, first trade at a station) -> a note completes -> instant chunk (~40-50%) of a related
  research item or standing gate.
- **Determinism:** derived, never rolled, exactly like `sim/contacts.go` ("Derived offers, never
  rolled ... a pure function of state"); the completed-note set is a sorted persisted list, so
  derivation moves no hash.
- **Interlock:** **derived mission offers + standing** (notes fire from the same events offers already
  read, and an offer stops existing when the world state closes the gap), **war front** (procurement
  deliveries credit military notes to the contest meter's logic), **industry/fitting** (what a note
  accelerates).
- **Pacing:** front-loaded expertise rewards; one note each, non-stacking (Civ's rule, §4.3).
- **Failure mode inherited:** Civ's checklist min-max (players chase boosts instead of playing) and
  stale objectives; derivation-with-fadeout is already Entieve's answer to staleness.

**C. Study jobs (proposal: a "study" job kind in Manufacturing).** Factorio/Anno research inside the
economy.
- **Loop:** queue a study at an `industry` station with reagents -> job consumes them over ticks ->
  unlocks a recipe/module/hull, optionally credits standing.
- **Determinism:** identical to manufacturing: "Timing is pure clock math
  (`Job.StartedAt`/`DoneAt`): zero RNG" (docs/SIM.md §3.3), inputs frozen at start like craft factors.
- **Interlock:** **industry** (it competes for the three global job slots, a real opportunity cost
  against manufacturing), **market** (reagent prices come off the books; Anno's item donation is the
  precedent, §4.2), **fitting** (the unlocks are gated by class quotas).
- **Pacing:** research speed = economy speed (Factorio, §4.1); throttle with slot competition and
  scarce reagents rather than timers.
- **Failure mode inherited:** Factorio's exponential science (production trivially outpaces content)
  and Anno's clock-wait boredom (§4.2); both are managed by scarcity, not by adding timers.

**D. Licences (proposal: "licence", spent from Standing).** GW2 hero points on the existing scalar.
- **Loop:** complete derived offers or get procurement fills credited -> standing rises -> spend
  standing on licences that open hulls, module classes or contact tiers.
- **Determinism:** ledger arithmetic and integer spends; the gate pattern already exists
  (`sim/system_mission.go`: `s.standing+1e-9 < def.MinStanding` refusing with "Requires standing ...";
  `s.standing += def.Reward.StandingGain` on completion).
- **Interlock:** **standing + contacts** (licences make explicit what the scalar already implies),
  **war front** (procurement is the standing faucet), **fitting/industry** (what a licence opens).
- **Pacing:** rep-paced and active; re-priceable exactly as ArenaNet did (§3.2).
- **Failure mode inherited:** standing inflation and checklist unlocks: GW2 had to cut 400 -> 250 and
  refund (§3.2, the studio's own post).

---

## Recommended operationalization *(inference / opinion)*

**Recommended stack *(opinion)*:** A is the spine: what the owner already likes (EVE's per-level
edges) made active, competing with nothing the game does not already simulate. B is the cheap
discovery layer that reuses derivation and makes growth legible in the UI (§6.1). C is the half to
add only if "tech tree" is meant literally: research as a production order, which Entieve's industry
can already express. D is optional glue between standing and unlocks. A + B satisfies "active" while
keeping every rule a pure function of tick and state; ship A first, watch whether the grind appears
(§2.2), then decide on B, C, D.

### What to grill the owner on first

**Grill Candidate A first** *(opinion)*: it is the direct active translation of the passive system he
already likes, and its answers decide whether B, C and D are needed at all. Hardest questions:

1. **Does skill growth *pace content* or *reward expertise*?** EVE does both: "Most items such as
   ships, modules ... will require some skills to be trained to a certain level before they can be
   used" (§1.1) gates gear, while per-level percentages edge it (§1.3); Entieve's craft level only
   edges (4%/2%, §2.6). If skills *gate*, growth paces content and must be hard to skip or buy (§6.3);
   if they only *edge*, growth rewards expertise and must stay small enough that the ungated game is
   still playable. Which one is Entieve buying?
2. **If growth is by use, what stops degenerate farming in a sandbox with no rivals?** Skyrim's
   "the lower the skill, the faster it increases" (§2.2), UO's bottomfeeding (§2.1) and X4's
   "longer they survive" (§2.5) all invite it. Committed points (Bannerlord, irreversible), quality
   of practice (Dwarf Fortress, §2.3), or decay (craft's 3600-tick rule, §2.6)? Pick one before any
   numbers exist.
3. **Where does respec live, and what does the currency rule say?** Free (MOO2, §4.5), item-cost
   (PoE, §5.1), full refund on request (Hades, §5.3), or locked (Bannerlord, §2.4)? And does a
   Proficiency bonus evaporate the moment you refit? Finally: is growth earned only in play, as a
   written rule (EA's "all progression will be earned through gameplay", §6.3), so no future version
   of this system quietly becomes a timer (§6.5) or a shop item (§6.2)?

---

## Source list

1. EVE support, *Skill Training*. https://support.eveonline.com/hc/en-us/articles/203217062-Skill-Training
2. EVE dev blogs (news pages, in order). https://www.eveonline.com/news/view/updates-to-skill-training ; https://www.eveonline.com/news/view/skill-trading-in-new-eden ; https://www.eveonline.com/news/view/exploring-the-character-bazaar-skill-trading ; https://www.eveonline.com/news/view/new-alpha-training-option ; https://www.eveonline.com/news/view/patch-notes-for-retribution-1
3. EVE University Wiki (community): *Skills and learning*, *Fitting skills*. https://wiki.eveuniversity.org/Skills_and_learning ; https://wiki.eveuniversity.org/Fitting_skills
4. Raph Koster, *Use-based systems* (2006). https://www.raphkoster.com/2006/07/18/use-based-systems/
5. Bethesda manuals (PDF): Skyrim. https://assets.ctfassets.net/rporu91m20dc/hH1HljyrjqewisSugMOm2/ab667e857bd67d39a814dc064d96ef8f/manual_skyrim-le_ps3_en-int.pdf ; Morrowind. https://assets.ctfassets.net/rporu91m20dc/6lHLSvBn8WoeCou2i0IAqa/77f3c4493ad458ab5a692d593f1af8a7/manual_mwgoty_pc_en-us.pdf
6. Kingdom Come Codex (community reader + raw game language file). https://kingdomcomecodex.github.io/ ; https://raw.githubusercontent.com/kingdomcomecodex/kingdomcomecodex.github.io/master/lang/en.html
7. Bay 12 dev log (DF), entry 03/19/2021 (Wayback 2021-03-21). https://www.bay12games.com/dwarves/ ; https://web.archive.org/web/20210321034714/http://www.bay12games.com/dwarves/ ; community wiki. https://dwarffortresswiki.org/index.php/Labor
8. Lo-Fi Games forum (Kenshi dev post). https://www.lofigames.com/phpBB3/viewtopic.php?p=429
9. TaleWorlds *Dev Blog 23/08/18* (Bannerlord), via Wayback. https://www.taleworlds.com/en/Games/Bannerlord/Blog/74 ; https://web.archive.org/web/2021id_/https://www.taleworlds.com/en/Games/Bannerlord/Blog/74
10. Egosoft X Wiki, *Crew* (X4). https://wiki.egosoft.com/X4%20Foundations%20Wiki/Manual%20and%20Guides/Objects%20in%20the%20Game%20Universe/Equipment/Crew/
11. WARFRAME wiki, *Focus*. https://wiki.warframe.com/w/Focus
12. ArenaNet news (3 pages). https://www.guildwars2.com/en/news/specializations-part-two-reward-tracks-and-elite-specializations/ ; https://www.guildwars2.com/en/news/heads-up-iteration-incoming/ ; https://www.guildwars2.com/en/news/earning-hero-points-in-world-vs-world/
13. FFXIV *Tales of Adventure*. https://na.finalfantasyxiv.com/tales_of_adventure/
14. Factorio wiki, *Research*. https://wiki.factorio.com/Research
15. Anno Union, *Scholars and Research*. https://www.anno-union.com/devblog-scholars-and-research/
16. 2K, Civ VI manual (PDF). http://downloads.2kgames.com/civilization/vi/manuals/eu/CIV_VI_25TH_ONLINE_MANUAL_ENG.pdf ; conflicting community: https://civ6bbg.github.io/ ; https://civilization.fandom.com/wiki/Boost_(Civ6)
17. Stellaris wiki (Paradox), *Technology*. https://stellaris.paradoxwikis.com/Technology
18. *Master of Orion II* manual scan. https://archive.org/details/Master.of.Orion.2
19. Grinding Gear Games, *Passive Skill Tree*. https://www.pathofexile.com/passive-skill-tree
20. 2K: Borderlands 4 Vex page ; Borderlands 3 update notes. https://borderlands.2k.com/borderlands-4/game-info/vault-hunters/vex/ ; https://borderlands.2k.com/borderlands-3/update-notes/
21. Supergiant: Hades *Nighty Night* patch notes ; patch 029 ; Hades FAQ. https://www.supergiantgames.com/blog/hades-the-nighty-night-update-patch-notes/ ; https://www.supergiantgames.com/blog/7/ ; https://www.supergiantgames.com/blog/hades-faq/
22. Bungie: *State of Destiny 2* ; *Road to The Witch Queen*. https://www.bungie.net/7/en/News/article/46504 ; https://www.bungie.net/7/en/News/article/50124
23. EA/DICE, *Battlefront II update* ; EACommunityTeam Reddit reply (Wayback-verified). https://www.ea.com/games/starwars/battlefront/news/pre-launch-update ; https://www.reddit.com/r/StarWarsBattlefront/comments/7cff0b/seriously_i_paid_80_to_have_vader_locked/dppum98/
24. Monolith/WB, *Shadow of War* FAQ. https://wbgamessupport.wbgames.com/hc/en-us/articles/1500002839382-Important-Free-Updates-Coming-to-Shadow-of-War-FAQ-04-02-2018
25. Blizzard (WoW): 9.1.5 preview, news roundup, update notes. https://worldofwarcraft.blizzard.com/en-us/news/23716422 ; https://worldofwarcraft.blizzard.com/en-us/news/23724371 ; https://worldofwarcraft.blizzard.com/en-us/news/23731242
26. Ubisoft statement on Valhalla XP boosts (Game Informer, secondary host). https://gameinformer.com/2020/12/16/assassins-creed-valhalla-sells-xp-boosts-now
27. This repo: `CONTEXT.md`, `docs/SIM.md`, `sim/system_industry.go`, `sim/system_mission.go`.
