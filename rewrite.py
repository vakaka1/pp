import re

def process():
    with open('scripts/install-server.sh', 'r') as f:
        content = f.read()

    # Split the file by `\n# =====...` blocks
    # We know that steps are surrounded by `# =====...` and we need to capture the step header and body.
    
    parts = re.split(r'\n# =============================================================================\nstep "(.*?)"\n# =============================================================================\n', content)
    
    # parts[0] is everything before the first step.
    new_content = parts[0]
    
    for i in range(1, len(parts), 2):
        step_name = parts[i]
        step_body = parts[i+1]
        
        # Check if it's already wrapped (e.g. step 1)
        if "show_progress" in step_body:
            # Just keep it as is, or we can standardize it
            if step_name == "Установка системных зависимостей":
                # Make it standard
                step_body = re.sub(r'\{.*?\} >/dev/null 2>&1 &\nshow_progress \$pid ".*?"\n\n(.*)',
                                   r'tmp_log=$(mktemp)\n{\n    if command -v apt-get &>/dev/null; then\n        export DEBIAN_FRONTEND=noninteractive\n        apt-get update -qq\n        apt-get install -y -qq curl nginx certbot python3-certbot-nginx libcap2-bin\n    elif command -v dnf &>/dev/null; then\n        dnf install -y curl nginx certbot python3-certbot-nginx libcap\n    elif command -v yum &>/dev/null; then\n        yum install -y curl nginx certbot python3-certbot-nginx libcap\n    fi\n} >"$tmp_log" 2>&1 &\nshow_progress $! "Установка системных зависимостей" || { cat "$tmp_log"; rm -f "$tmp_log"; die "Ошибка на этапе: Установка системных зависимостей"; }\ncat "$tmp_log"\nrm -f "$tmp_log"\n\n\1',
                                   step_body, flags=re.DOTALL)
                pass
        
        # For all other steps, we need to wrap their body until the next `# =====...` or EOF.
        # But wait, there are also `ok "Зависимости установлены"` at the end of bodies which we might want OUTSIDE the background block?
        # Actually, no, if they print `ok`, we can just capture them in $tmp_log, and they will be printed after progress finishes!
        # `show_progress` hides output during the animation, then `cat "$tmp_log"` dumps it! This is perfectly fine.
        
        # However, the problem is variables set inside the subshell are lost.
        # Let's check if any step sets variables that are used in subsequent steps.
        # Step 2: FRONTEND_ARCHIVE_NAME, FRONTEND_URL.
        # Wait, if FRONTEND_TMP is used, it's local. But if variables are set and exported, they are lost!
        # Let's check step 2.
        pass

process()
