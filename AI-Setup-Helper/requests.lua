local R = {}

local json = require("json")

local function direct_payload(data, api_key)
    local url = "https://openrouter.ai/api/v1/chat/completions"
    local headers = {
        ["Authorization"] = "Bearer " .. api_key,
        ["Content-Type"] = "application/json",
    }

    ac.debug("LLM Data", data)

    -- According to research I did, the ranking of the models
    -- based on performance/price ratio is:
    -- deepseek-v4-flash
    -- deepseek-v4-pro
    -- Gemini 2.5 Flash
    local model = "deepseek/deepseek-v4-flash"
    local prompt = "You are an expert Assetto Corsa race engineer generating car setups.\n" ..
        "You will be given a JSON object with car, track, and condition data. Ignore any field that is nil/null. If there is a value that should be considered into the setup, and is important.\n" ..
        "Respond ONLY with a JSON array of {\"n\":..., \"v\":...} objects, where n is name and v is value, one per field you are changing. \n" ..
        "Only modify setup parameters that already exist in the provided setup. Never invent new parameter names.\n" ..
        "Do not include markdown formatting or any text outside the JSON array.\n" ..
        "Stay within each field's min/max range if provided.\n\n" ..
        "- If \"oversteer\" or \"understeer\" are true, adjust the setup to reduce the one that is true.\n" ..
        "- If both are false, do not apply any oversteer/understeer-specific correction; base the setup purely on the " ..
        "other provided data (track, car, temps, weather, fuel).\n" ..
        "Assume the current setup is only a starting point, not an optimized setup. Analyze every adjustable parameter and modify any parameter that would improve the setup for the given track and conditions. Leave a parameter unchanged only if you determine it is already near its optimal value.\n" ..
        "You should return a modified setup. That setup should be a base stable setup, target a predictable, confidence-inspiring setup suitable for most drivers rather than an aggressive qualifying setup.\n" ..
        "Data:\n" .. data

    local payload = json.encode({
        model = model,
        messages = {
            {
                role = "user",
                content = prompt
            }
        },
        reasoning = {
            enabled = false
        }
    })

    return url, headers, payload
end

local function common_payload(data, api_key)
    local url = ""

    if CachedMachineHash == nil then
        getHash()
    end

    local headers = {
        ["Content-Type"] = "application/json",
        ["X-Machine-Hash"] = CachedMachineHash
    }

    ac.debug("LLM Data", data)
    
    return url, headers, data
end

--- Makes the request to the chosen provider
--- @param common   boolean
---@param data      string
---@param api_key   string
---@param callback function
function R.make_request(common, data, api_key, callback)
    local url, headers, payload
    
    if common then
        url, headers, payload = common_payload(data, api_key)
    else
        url, headers, payload = direct_payload(data, api_key)
    end

    if type(payload) ~= "string" then
        callback("Failed to encode request payload.", false)
        return
    end

    web.request(
        "POST",
        url,
        headers,
        payload,
        function(err, response)
            if err then
                if response then
                    if response.status == 429 then
                        callback("Rate limit exceeded. Try again later.", false)
                    elseif response.status == 401 then
                        callback("Unauthorized. Check your API key.", false)
                    elseif response.status == 502 then
                        callback("Chosen provider is down. Try again later.", false)
                    elseif response.status == 402 then
                        callback("One of us screwed up. And ran out of credits. (If you use the common provider, it was me, sorry)", false)
                    elseif response.status == 413 then
                        callback("Body size too large", false)
                    else
                        callback("Request failed with status: " .. response.status, false)
                    end
                else
                    callback(err, false)
                end

                return
            end
            
            local content = ""

            if common then
                content = response.body
            else
                local ok, body = pcall(json.decode, response.body)
                if not ok or not body then
                    callback("Invalid JSON response from provider.", false)
                    return
                end
                if not body
                    or not body.choices
                    or not body.choices[1]
                    or not body.choices[1].message then
                    callback("Invalid response from provider.", false)
                    return
                end

                ac.debug("LLM Response", body.choices[1].message.content)
                content = body.choices[1].message.content
            end

            -- Clean json
            content = content:gsub("^```json%s*", ""):gsub("^```%s*", ""):gsub("```%s*$", "")
            callback(content, true)
        end
    )

end

return R
