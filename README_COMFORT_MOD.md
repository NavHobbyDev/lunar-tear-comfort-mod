# Lunar Tear — Comfort Mod  
  
Special thanks to:  
  
- [Walter-Sparrow/lunar-tear](https://github.com/Walter-Sparrow/lunar-tear)  
- [JCalandre/lunar-tear](https://github.com/JCalandre/lunar-tear)  
- [Lycheenut/lunar-tear](https://github.com/Lycheenut/lunar-tear)  
  
> **Important**  
> For this mod to work correctly, the masterdata file **must** be patched with the  
> [comfort-mod](https://github.com/NavHobbyDev/lunar-tear-masterdata-patcher/tree/comfort-mod)  
> version of the patcher.  
> (The config in this branch is different from the main patcher.)
  
---  
  
## What this mod changes  
  
- Numerous bug fixes and stability improvements  
- **Story-gated gacha**    
  A character’s gacha becomes available as soon as you meet them in the story    
  (collaboration banners open purely by story progression)  
- **Single-character gacha pools**    
  Each banner contains only costumes of one character (all except Dark)    
  + weapons matching that character’s weapon affinity    
  (if every costume is one-handed, only one-handed weapons appear)    
  Collaboration banners are an exception and may contain multiple characters  
- **No duplicates until collection is complete**    
  Duplicates are excluded until every costume or weapon in the pool has been obtained  
- **Pity conversion**    
  Every 100 pity coins are automatically converted into:  
  - 1 000 paid gems    
  - 100 Mama Points    
  - 10 Countdown Resurrected Event Medals    
  (delivered to the mailbox)  
- **Premium shop uses only paid gems**    
  All items are purchased with paid gems obtained exclusively from gacha    
  (free gems are never credited for these purchases — only materials)  
- **Unlimited daily / weekly / monthly shop limits**    
  Refreshing shop items no longer exhaust their purchase counters  
- **Quest rewards ×10**  
- **Gems from quest missions and regular missions ×10**  
- **Missions are challenge-only**    
  Missions behave like permanent achievements  
- **PvP Simulation**    
  Fully automatic battles using your strongest deck among those previously used or cleared in any quest. The outcome depends purely on deck power. If power levels are within ±25%, the result is randomized. 
  You cannot influence the outcome — you can only watch the results table.    
  20 auto-battles per day  
- **Daily seasonal rewards**    
  Seasonal rewards for PvP, Subjugation and Fate Board are delivered to the mailbox every day  
- **Extra options in Notices window**
  - Switch master-data file without restarting the server  
  - Skip missions to unlock Hidden Stories  
  - Auto-complete quest missions (all three clear requirements)  
  - Send any item to the mailbox by ID without a save editor    
    (requires exact Possession ID + Item ID — no safety checks are performed, incorrect values can corrupt the save)  
- **Memoir bonuses scale with rarity**    
  Higher rarity → more bonus slots (max rarity = maximum bonuses)  
- **Memoir upgrade chance**    
  Chance to improve all 5 bonuses at once when upgrading a Memoir  
- **Memoir drops**    
  Always drop as a set of 5 unique pieces (no repeats inside a set when more than one set exists)  
- **Lost Items**    
  Always grant a random amount of free gems  
- **Stamina system**    
  Stamina is never consumed; stamina bottles are converted into gold  
- **Premium tickets**    
  All premium tickets are converted into free gems
  
---  

<details>
<summary><b>Screenshot Extra options</b></summary>
<br>

<p align="center">
  <img src="https://github.com/user-attachments/assets/b2adb7df-e1ff-4856-96df-579aef61f5d3" alt="Screenshot">
</p>

</details>

<details>
<summary><b>Switching between masterdata variants in the client</b></summary>

<br>

- You do **not** need to rename the files. Simply place all variants into the `release` folder — the client will automatically detect every available variant.
- If you create your own variants, the main rule is that the file `20240404193219.bin.e` must contain the full content from **all** variants.  
  In the client this file is displayed as **«All»**.
- You can create custom variants and name them following the pattern:  
  `20240404193219_YourName.bin.e`
- The selection is independent for each player on the server — every player can use their own chosen masterdata.

</details>

---

## License and attribution

**This repository** (modifications and comfort-mod changes) is released under the [MIT License](LICENSE).

The base code is derived from:
- [Walter-Sparrow/lunar-tear](https://github.com/Walter-Sparrow/lunar-tear) (MIT License, Copyright (c) 2026 Ilya Groshev)
- [JCalandre/lunar-tear](https://github.com/JCalandre/lunar-tear)
- [Lycheenut/lunar-tear](https://github.com/Lycheenut/lunar-tear)

Upstream code remains the work of its original authors. This repository does not claim to re-license their work beyond the terms of the original MIT license.

Documentation and some code changes were written with AI assistance;  
the maintainer reviewed and verified the final result.

If you are an upstream author and want a different credit line, license note, or removal, open an issue or contact the maintainer.

---
  
### Disclaimer  
  
Not affiliated with Square Enix, Applibot, or any official Lunar Tear server project.    
This mod does not distribute game clients, assets, or official binaries.    
You must provide your own data files. Use at your own risk.  
