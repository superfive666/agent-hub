package org.agenthub.app.ui.components

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material3.LocalTextStyle
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import org.agenthub.app.ui.theme.UiFont
import org.agenthub.app.ui.theme.tokens

/**
 * **控件全面胶囊化。** 按钮、输入框、chip、分段器、头像一律全圆角 ——
 * 这是整套设计里最强的形状记号，破一个就露馅。
 */
val Pill = RoundedCornerShape(percent = 50)

/** 触控目标 ≥ 44dp（设计语言 §4）。手指不是鼠标指针。 */
val MinTouch = 44.dp

enum class ButtonTone {
    /** 主按钮：薄荷渐变（暗色下紫洋红）。一屏最多一个 */
    Primary,

    /** 次级：玻璃面 + 描边 */
    Default,

    /** 幽灵：只有文字，用在工具条上 */
    Ghost,

    /** 危险动作。删除、吊销凭证这些 */
    Danger,
}

@Composable
fun PillButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    tone: ButtonTone = ButtonTone.Default,
    enabled: Boolean = true,
    leading: (@Composable () -> Unit)? = null,
    trailing: (@Composable () -> Unit)? = null,
) {
    val t = tokens()
    val bg: Brush = when (tone) {
        ButtonTone.Primary -> t.priGrad
        ButtonTone.Default -> SolidColor(t.chipBg)
        ButtonTone.Ghost -> SolidColor(Color.Transparent)
        ButtonTone.Danger -> SolidColor(t.alertSoft)
    }
    val ink = when (tone) {
        ButtonTone.Primary -> t.priInk
        ButtonTone.Danger -> t.alert
        ButtonTone.Ghost -> t.ink2
        ButtonTone.Default -> t.ink
    }
    val border = when (tone) {
        ButtonTone.Default -> BorderStroke(1.dp, t.hair)
        ButtonTone.Danger -> BorderStroke(1.dp, t.alert.copy(alpha = 0.4f))
        else -> null
    }

    Row(
        modifier = modifier
            .defaultMinSize(minHeight = MinTouch)
            .clip(Pill)
            .background(bg, Pill)
            .then(if (border != null) Modifier.border(border, Pill) else Modifier)
            .alpha(if (enabled) 1f else 0.45f)
            .clickable(enabled = enabled, onClick = onClick)
            .padding(horizontal = 20.dp, vertical = 12.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.CenterHorizontally),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        leading?.invoke()
        Text(
            text = text,
            color = ink,
            style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.Bold),
        )
        trailing?.invoke()
    }
}

/**
 * Chip。`tone` 是**语义**不是配色：
 * 「人类」chip 永远是暖橘、「主 agent」是青、「关注」是虚线。
 */
enum class ChipTone { Neutral, Human, Agent, Watcher, Alert, Warn }

@Composable
fun Chip(
    text: String,
    modifier: Modifier = Modifier,
    tone: ChipTone = ChipTone.Neutral,
) {
    val t = tokens()
    val (bg, ink) = when (tone) {
        ChipTone.Human -> t.humanSoft to t.humanInk
        ChipTone.Agent -> t.agentSoft to t.agentInk
        ChipTone.Watcher -> Color.Transparent to t.agentInk
        ChipTone.Alert -> t.alertSoft to t.alert
        ChipTone.Warn -> t.warnSoft to t.warn
        ChipTone.Neutral -> t.chipBg to t.ink2
    }
    Box(
        modifier = modifier
            .clip(Pill)
            .background(bg, Pill)
            .then(
                // 关注者的虚线在 chip 上退化成实线细边（虚线在 12px 的胶囊上
                // 只会糊成一条脏边）—— 语义由文字「关注」承担，
                // 虚线那一重留给头像，那里的尺寸撑得住。
                if (tone == ChipTone.Watcher) {
                    Modifier.border(1.dp, t.agent.copy(alpha = 0.45f), Pill)
                } else {
                    Modifier
                },
            )
            .padding(horizontal = 10.dp, vertical = 5.dp),
    ) {
        Text(
            text = text,
            color = ink,
            // labelSmall 带 1sp 字距（那是给全大写拉丁标签的）。
            // chip 里多是中文，字距会把两个汉字拉散，所以清零。
            style = MaterialTheme.typography.labelSmall.copy(
                fontWeight = FontWeight.Bold,
                letterSpacing = 0.sp,
            ),
        )
    }
}

/** 分段器。web 上那个 `.seg`，选中项是薄荷底。 */
@Composable
fun Seg(
    options: List<Pair<String, String>>,
    value: String,
    onValueChange: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val t = tokens()
    Row(
        modifier = modifier
            .clip(Pill)
            .background(t.insetBg, Pill)
            .border(1.dp, t.insetBd, Pill)
            .padding(3.dp),
        horizontalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        options.forEach { (v, label) ->
            val active = v == value
            Box(
                modifier = Modifier
                    .weight(1f)
                    .clip(Pill)
                    .then(if (active) Modifier.background(t.priGrad, Pill) else Modifier)
                    .clickable { onValueChange(v) }
                    .padding(vertical = 9.dp),
                contentAlignment = Alignment.Center,
            ) {
                Text(
                    text = label,
                    color = if (active) t.priInk else t.ink2,
                    style = MaterialTheme.typography.labelMedium,
                )
            }
        }
    }
}

/**
 * 输入框。
 *
 * 用 [BasicTextField] 而不是 Material 的 TextField：后者自带一整套
 * label 浮动、下划线、容器配色，全都要覆盖掉，覆盖到最后剩下的还不如自己画。
 */
@Composable
fun PillField(
    value: String,
    onValueChange: (String) -> Unit,
    modifier: Modifier = Modifier,
    placeholder: String = "",
    singleLine: Boolean = true,
    visualTransformation: VisualTransformation = VisualTransformation.None,
    keyboardOptions: androidx.compose.foundation.text.KeyboardOptions =
        androidx.compose.foundation.text.KeyboardOptions.Default,
    shape: RoundedCornerShape = Pill,
    trailing: (@Composable () -> Unit)? = null,
) {
    val t = tokens()
    Row(
        modifier = modifier
            .clip(shape)
            .background(t.insetBg, shape)
            .border(1.dp, t.insetBd, shape)
            .defaultMinSize(minHeight = MinTouch)
            .padding(PaddingValues(horizontal = 18.dp, vertical = 12.dp)),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(modifier = Modifier.weight(1f)) {
            if (value.isEmpty() && placeholder.isNotEmpty()) {
                Text(
                    text = placeholder,
                    color = t.ink3,
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
            BasicTextField(
                value = value,
                onValueChange = onValueChange,
                singleLine = singleLine,
                visualTransformation = visualTransformation,
                keyboardOptions = keyboardOptions,
                cursorBrush = SolidColor(t.agent),
                textStyle = LocalTextStyle.current.merge(
                    TextStyle(color = t.ink, fontFamily = UiFont, fontWeight = FontWeight.Medium),
                ),
                modifier = Modifier.fillMaxWidth(),
            )
        }
        trailing?.invoke()
    }
}
