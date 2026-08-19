# AI-Setup-Helper-AC

An Assetto Corsa Lua app to use AI to generate setups for you

Very important to note before anything, this is not a replacement for a brain. The goal is not to use the app blindly
and trust AI slop setups, while your brain goes smooth. 

It was made for people who have no idea in which direction to go when starting a setup.

The idea for the app is to get that direction from the AI, but still, most likely you will still need to 
fine-tune the setup to make
it good, but by this point you should only need to tweak some fields (even if you have no clue what, read the 
descriptions for the fields in the game).

## Usage

Click on the 'Request' button to get the setup. If you wish to include the fields available, click on the boxes.
The current values are the default, some have sliders for you to choose custom values.

There are also options to indicate the AI to try and fix oversteer or understeer.

The small text box below the 'Request' button, informs you of the current state

- if yellow, the request is pending and being processed
- if green, the request was received and applied successfully
- red means something wrong. The message
may indicate what (if it is not something to solve on your end, it may be more generic).

Some errors include:

- *Rate limit exceeded* → you sent too many request in a short span of time, wait briefly until it resets
- *Unauthorized* → something in the configuration is wrong and either the common app (explained below) or the provider
are blocking the usage
- *Chosen provider is down* → the AI provider is down so no AI setups
- *Ran out of credits* → no more money on the AI provider
- *Body size too large* → means that somehow (it should only happen to funny people trying to exploit the shared app)
the contents send, are much bigger than they should
- *Invalid (something) from provider* → most likely the AI hallucinated and returned garbage

## Modes

There are 2 modes to use the app

### Common

This is the default mode. All requests go to a shared app.

#### Pros

- No configurations needed
- No money involved
- Some setups may be cached, so it can be faster

#### Cons

- The app is hosted on a free tier of a provider, which means sometimes it shuts off.
The first request can take longer while it is booting up.
- If people try to abuse it, I will shut it down
- There are rate limits, so you make to many requests in a short period of time, you may be temporarily blocked.
- You are dependent of me keeping up with the AI provider costs, and I am not Santa Claus, if they start asking me too
much (or better anything at all), and there are no donations, I will turn it off also :)

### Personal

This is the other one, it requires *some* configuration (creating an account and copy-pasting some text :| )

#### Pros

- You are only depend on the AI provider and not me and my shared app
- May be faster (fewer requests made)
- You can customise it better, you can change the prompt or the models in the `AI-Setup-Helper.lua` file

#### Cons

- Requires some configuration (not much, but more than there should be for an Assetto Corsa mod)
- If you use it too much, the provider may start asking you for money (some don't ask for low usage)
- If you screw up any configuration or any changes you make, that's on you.

## Set Up Personal Mode